package findmy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Window struct {
	PID      int    `json:"pid"`
	WindowID int    `json:"windowID"`
	Layer    int    `json:"layer"`
	Title    string `json:"title"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	OnScreen bool   `json:"onScreen"`
}

type TextLine struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
}

type Person struct {
	Name      string `json:"name"`
	Location  string `json:"location,omitempty"`
	Staleness string `json:"staleness,omitempty"`
	Distance  string `json:"distance,omitempty"`
	Address   string `json:"address,omitempty"`
	NameY     int    `json:"-"` // image pixel Y of the name row (for click targeting)
	NameX     int    `json:"-"` // image pixel X of the name row
}

func helper() string {
	if env := os.Getenv("FINDMY_HELPER"); env != "" {
		return env
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "findmy-helper")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if path, err := exec.LookPath("findmy-helper"); err == nil {
		return path
	}
	return "findmy-helper"
}

func runHelper(args ...string) ([]byte, error) {
	cmd := exec.Command(helper(), args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

type Permissions struct {
	ScreenRecording bool `json:"screenRecording"`
	Accessibility   bool `json:"accessibility"`
}

// CheckPermissions returns nil when both Screen Recording (for screencapture)
// and Accessibility / event-posting (for CGEvent clicks) are granted to the
// helper binary. It uses the helper's `permissions` subcommand, which probes
// via SCShareableContent rather than trusting CGPreflight*Access alone — TCC
// state is often stale for CLI binaries across rebuilds, and the preflight
// calls return false-negatives that would otherwise cause screencapture to
// hang or click to silently no-op.
func CheckPermissions() (Permissions, error) {
	out, err := runHelper("permissions")
	if err != nil {
		return Permissions{}, fmt.Errorf("helper permissions: %w", err)
	}
	var p Permissions
	if err := json.Unmarshal(out, &p); err != nil {
		return Permissions{}, fmt.Errorf("decode permissions: %w", err)
	}
	return p, nil
}

func requirePermissions(needClick bool) error {
	p, err := CheckPermissions()
	if err != nil {
		return err
	}
	var missing []string
	if !p.ScreenRecording {
		missing = append(missing, "Screen Recording")
	}
	if needClick && !p.Accessibility {
		missing = append(missing, "Accessibility")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"missing permission(s) for the host process: %s. Grant in System Settings → Privacy & Security → %s, then fully quit and relaunch this terminal (TCC is read once at process start).",
		strings.Join(missing, ", "), strings.Join(missing, " / "),
	)
}

func Activate() error {
	script := `tell application "FindMy" to activate`
	return exec.Command("osascript", "-e", script).Run()
}

// hintDedicatedSpace prints a one-time hint if FindMy is on the current Space,
// recommending the user move it to a dedicated Space for less disruption.
func hintDedicatedSpace() {
	ls := GetAppStrings()
	out, err := runHelper("window", "--owner", ls.WindowOwner)
	if err != nil {
		return
	}
	var wins []Window
	if err := json.Unmarshal(out, &wins); err != nil {
		return
	}
	for _, w := range wins {
		if w.Layer == 0 && w.Height > 100 && w.OnScreen {
			fmt.Fprintf(os.Stderr, "hint: %s is on your current desktop. For less disruption, assign it to a dedicated Space:\n      right-click %s in Dock → Options → Assign To → Desktop on Display 2\n", ls.WindowOwner, ls.WindowOwner)
			return
		}
	}
}

// rememberFrontApp returns the bundle identifier of the current frontmost app,
// so we can restore focus after switching to FindMy's Space.
func rememberFrontApp() string {
	out, err := exec.Command("osascript", "-e",
		`tell application "System Events" to get bundle identifier of first process whose frontmost is true`).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// restoreFrontApp reactivates a previously frontmost app by bundle identifier,
// causing macOS to switch back to its Space automatically.
func restoreFrontApp(bundleID string) {
	if bundleID == "" || bundleID == "com.apple.findmy" {
		return
	}
	_ = exec.Command("osascript", "-e",
		fmt.Sprintf(`tell application id %q to activate`, bundleID)).Run()
}

func SwitchTab(name string) error {
	ls := GetAppStrings()
	script := fmt.Sprintf(
		`tell application "System Events" to tell process "FindMy" to click menu item %q of menu %q of menu bar 1`,
		name, ls.ViewMenu,
	)
	return exec.Command("osascript", "-e", script).Run()
}

func MainWindow() (*Window, error) {
	ls := GetAppStrings()
	out, err := runHelper("window", "--owner", ls.WindowOwner)
	if err != nil {
		return nil, fmt.Errorf("helper window: %w", err)
	}
	var wins []Window
	if err := json.Unmarshal(out, &wins); err != nil {
		return nil, fmt.Errorf("decode windows: %w", err)
	}
	// Prefer on-screen windows, but accept off-screen (other Space) too.
	// screencapture -l works across Spaces.
	var best *Window
	for i := range wins {
		w := &wins[i]
		if w.Layer != 0 || w.Height <= 100 {
			continue
		}
		if w.OnScreen {
			return w, nil // on-screen is ideal
		}
		if best == nil {
			best = w // off-screen fallback (FindMy on another Space)
		}
	}
	if best != nil {
		return best, nil
	}
	return nil, fmt.Errorf("no %s window found (open the app first)", ls.WindowOwner)
}

// Capture writes the FindMy window's content to dest using `screencapture -l`,
// which targets the window by ID and captures actual content rather than the
// screen rect. Region capture (`-R x,y,w,h`) would grab whatever is topmost at
// those coordinates and pollute the OCR with terminal/desktop content when
// FindMy isn't strictly frontmost.
//
// Capture fails with a friendly error when the display is asleep or the
// window's backing store hasn't been populated yet (Catalyst quirk after
// rapid focus changes). Both produce "could not create image from window"
// or a tiny all-black PNG.
func Capture(w *Window, dest string) error {
	cmd := exec.Command("/usr/sbin/screencapture", "-x", "-l", fmt.Sprintf("%d", w.WindowID), "-t", "png", dest)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return diagnoseCaptureFailure(err)
	}
	if info, err := os.Stat(dest); err == nil && info.Size() < 5_000 {
		return fmt.Errorf("captured image suspiciously small (%d bytes); display may be asleep — wake it with the keyboard", info.Size())
	}
	return nil
}

func diagnoseCaptureFailure(err error) error {
	if isDisplayAsleep() {
		return fmt.Errorf("display is asleep; wake it with the keyboard and re-run (%w)", err)
	}
	return fmt.Errorf("screencapture: %w (FindMy may not be fully painted; try again or click into FindMy first)", err)
}

func isDisplayAsleep() bool {
	out, err := exec.Command("ioreg", "-c", "IODisplayWrangler").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), `"CurrentPowerState" = 1`) ||
		strings.Contains(string(out), `"CurrentPowerState" = 0`)
}

func OCR(image string) ([]TextLine, error) {
	out, err := runHelper("ocr", image)
	if err != nil {
		return nil, fmt.Errorf("helper ocr: %w", err)
	}
	var lines []TextLine
	if err := json.Unmarshal(out, &lines); err != nil {
		return nil, fmt.Errorf("decode ocr: %w", err)
	}
	return lines, nil
}

func Click(x, y int) error {
	_, err := runHelper("click", fmt.Sprintf("%d", x), fmt.Sprintf("%d", y))
	return err
}

// Scroll sends a scroll-wheel event at the given screen coordinates.
// dy < 0 scrolls down, dy > 0 scrolls up.
func Scroll(x, y, dy int) error {
	_, err := runHelper("scroll", fmt.Sprintf("%d", x), fmt.Sprintf("%d", y), fmt.Sprintf("%d", dy))
	return err
}

// wakeDisplay nudges the display awake by holding a 3-second user-activity
// assertion. Needed for headless / closed-lid use with a dummy USB-C display:
// the dummy plug enables clamshell mode but macOS still idle-sleeps it, and
// WindowServer stops compositing when its only display is asleep — which
// makes `screencapture -l <windowID>` return "could not create image from
// window". We fire-and-forget; caffeinate self-terminates after 3s, which
// covers PreparePeople's ~2s of activate+sleep before the capture.
func wakeDisplay() {
	cmd := exec.Command("caffeinate", "-u", "-t", "3")
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}

// PreparePeople activates FindMy on its Space, selects the People tab, and
// returns the window metadata. The caller is responsible for capturing the
// screenshot while FindMy is still frontmost — screencapture -l does NOT
// work cross-Space on macOS Sequoia (the backing store is empty).
//
// Call RestoreUserSpace() after the capture to switch back.
func PreparePeople() (*Window, error) {
	if err := requirePermissions(false); err != nil {
		return nil, err
	}

	wakeDisplay()

	// Check if FindMy is already on-screen (e.g. virtual display).
	// If so, skip activation to avoid stealing focus / switching Spaces.
	w, _ := MainWindow()
	if w != nil && w.OnScreen {
		// Window is on a visible display — just switch tab via menu.
		_ = SwitchTab(GetAppStrings().PeopleTab)
		time.Sleep(600 * time.Millisecond)
		return MainWindow()
	}

	// Window is off-screen (other Space) or not open — activate.
	hintDedicatedSpace()
	previousApp = rememberFrontApp()
	if err := Activate(); err != nil {
		return nil, err
	}
	time.Sleep(900 * time.Millisecond)
	frontScript := `tell application "System Events" to tell process "FindMy" to set frontmost to true`
	_ = exec.Command("osascript", "-e", frontScript).Run()
	_ = SwitchTab(GetAppStrings().PeopleTab)
	time.Sleep(1100 * time.Millisecond)

	return MainWindow()
}

// previousApp holds the bundle ID of the app that was frontmost before FindMy
// was activated. Set by PreparePeople, used by RestoreUserSpace.
var previousApp string

// RestoreUserSpace switches back to the app/Space that was active before
// PreparePeople was called.
func RestoreUserSpace() {
	restoreFrontApp(previousApp)
}

// ParsePeople groups OCR lines from the People sidebar into Person records.
// The sidebar layout (in image pixels) has three bands:
//
//	avatar:     x ≈   0–200   (round photo with initials — OCR noise lives here)
//	text:       x ≈ 240–550   (name and location/staleness)
//	distance:   x ≈ 580–700   ("1,971 mi" right-aligned to row top)
//
// We discard the avatar band entirely (it produces low-confidence fragments
// like "Is" or "rk" from initials and shadows that otherwise get misread as
// person names), then walk the remaining lines top-to-bottom.
func ParsePeople(lines []TextLine, sidebarRightPx, textColMinPx, topMarginPx int) []Person {
	rows := make([]TextLine, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if l.X+l.Width/2 >= sidebarRightPx {
			continue
		}
		if l.Y < topMarginPx {
			continue
		}
		if l.X < textColMinPx {
			continue
		}
		rows = append(rows, l)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Y == rows[j].Y {
			return rows[i].X < rows[j].X
		}
		return rows[i].Y < rows[j].Y
	})
	rows = mergeWrappedContinuations(rows)

	skip := GetAppStrings().SkipWords()

	var people []Person
	var current *Person
	for _, l := range rows {
		txt := strings.TrimSpace(l.Text)
		if skip[txt] {
			continue
		}
		if isDistance(txt) {
			if current != nil {
				current.Distance = txt
			}
			continue
		}
		if current == nil || current.Location != "" {
			people = append(people, Person{Name: txt, NameY: l.Y, NameX: l.X})
			current = &people[len(people)-1]
			continue
		}
		loc, stale := splitLocationStaleness(txt)
		current.Location = loc
		current.Staleness = stale
	}
	return people
}

// mergeWrappedContinuations folds OCR lines that Vision split across two
// visual rows because of a long "City, ST • 2 min. ago" string. The
// telltale: the previous row contains the " • " separator and the next row
// is within ~35px below it and looks like a relative-time suffix.
func mergeWrappedContinuations(rows []TextLine) []TextLine {
	out := make([]TextLine, 0, len(rows))
	for _, l := range rows {
		if n := len(out); n > 0 {
			prev := &out[n-1]
			gap := l.Y - (prev.Y + prev.Height)
			if gap < 12 && strings.Contains(prev.Text, "•") && looksLikeTimeSuffix(l.Text) {
				prev.Text = prev.Text + " " + strings.TrimSpace(l.Text)
				if l.Y+l.Height > prev.Y+prev.Height {
					prev.Height = (l.Y + l.Height) - prev.Y
				}
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

func looksLikeTimeSuffix(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	for _, pattern := range GetAppStrings().TimeSuffixes {
		if t == pattern || strings.HasSuffix(t, pattern) || strings.Contains(t, pattern) {
			return true
		}
	}
	return false
}

func isDistance(s string) bool {
	s = strings.ToLower(s)
	for _, suffix := range []string{" mi", " km", " ft", " m", " yd"} {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func splitLocationStaleness(s string) (location, staleness string) {
	if idx := strings.Index(s, "•"); idx >= 0 {
		return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+len("•"):])
	}
	return s, ""
}

// DetailAddress clicks on a person's row in the sidebar so FindMy zooms the
// map to their location, then captures the window and extracts the nearest
// street name from the map labels. This provides more precise location than
// the coarse city name from the sidebar.
//
// Requires Accessibility permission for the click.
// sidebarShot is the people screenshot (used to compute image-to-screen scale).
// detailDest is the path where the detail screenshot will be saved.
func DetailAddress(w *Window, person *Person, sidebarShot, detailDest string) (string, error) {
	if err := requirePermissions(true); err != nil {
		return "", err
	}

	// If FindMy is on a virtual display (onScreen but not the main display),
	// CGEvent clicks work directly at its screen coordinates — no need to
	// activate or switch Spaces. Only activate if the window is off-screen
	// (on another Space).
	if !w.OnScreen {
		_ = Activate()
		time.Sleep(1200 * time.Millisecond)
		frontScript := `tell application "System Events" to tell process "FindMy" to set frontmost to true`
		_ = exec.Command("osascript", "-e", frontScript).Run()
		time.Sleep(500 * time.Millisecond)
		defer RestoreUserSpace()
	}

	scale := computeScale(w, sidebarShot)
	sidebarRightPx := int(340 * scale)

	// Click the person's name row in the sidebar to zoom the map.
	screenX := w.X + int(float64(person.NameX)/scale) + 20
	screenY := w.Y + int(float64(person.NameY)/scale) + 8
	if err := Click(screenX, screenY); err != nil {
		return "", fmt.Errorf("click person row: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Step 1: Click the map center to hit the pin → shows a popup bubble.
	mapCenterX := w.X + int(float64(sidebarRightPx)/scale) + (w.Width-int(float64(sidebarRightPx)/scale))/2
	mapCenterY := w.Y + w.Height/2
	_ = Click(mapCenterX, mapCenterY)
	time.Sleep(1500 * time.Millisecond)

	// Step 2: Capture, OCR, look for the popup bubble. If the detail card
	// is already open (e.g. from a previous run), return the address.
	// Otherwise, find the popup's (i) button and click it.
	for attempt := 0; attempt < 3; attempt++ {
		if err := Capture(w, detailDest); err != nil {
			return "", fmt.Errorf("detail capture: %w", err)
		}
		lines, err := OCR(detailDest)
		if err != nil {
			return "", fmt.Errorf("detail ocr: %w", err)
		}

		// Check if detail card is already showing.
		if addr := parseDetailPane(lines, person, sidebarRightPx); addr != "" {
			return addr, nil
		}

		// Find the popup bubble and click to the right of it (the (i) button).
		if clickX, clickY, ok := findPopupInfoButton(lines, person, w, sidebarRightPx, scale); ok {
			_ = Click(clickX, clickY)
		} else {
			// No popup found — retry clicking the map center.
			_ = Click(mapCenterX, mapCenterY)
		}
		time.Sleep(1500 * time.Millisecond)
	}

	return "", fmt.Errorf("detail card did not appear after 3 attempts")
}

// findPopupInfoButton locates the (i) button in any FindMy popup bubble on
// the map. The popup may show a different name than the target person (e.g.
// "Moi" when pins overlap). We find the rightmost text in the map area and
// click just past it, where the (i) button sits.
func findPopupInfoButton(lines []TextLine, person *Person, w *Window, sidebarRightPx int, scale float64) (screenX, screenY int, found bool) {
	// Find the text cluster in the map area that looks like a popup:
	// small text lines outside the sidebar, not map labels (which are ALL-CAPS).
	var bestLine TextLine
	bestRight := 0
	for _, l := range lines {
		if l.X < sidebarRightPx {
			continue
		}
		txt := strings.TrimSpace(l.Text)
		if len(txt) < 3 || txt == "3D" || txt == "N" || txt == "+" {
			continue
		}
		// Map labels are ALL-CAPS; popup text is mixed-case.
		if txt == strings.ToUpper(txt) && len(txt) > 5 {
			continue
		}
		right := l.X + l.Width
		if right > bestRight {
			bestRight = right
			bestLine = l
		}
	}
	if bestRight == 0 {
		return 0, 0, false
	}
	// Click ~25px past the rightmost text edge (where the (i) sits).
	imgX := bestRight + int(25*scale)
	imgY := bestLine.Y + bestLine.Height/2
	return w.X + int(float64(imgX)/scale), w.Y + int(float64(imgY)/scale), true
}

// computeScaleFromWindow takes a quick screenshot of the window to determine
// the image-to-screen scale factor. This handles both @1x (virtual display)
// and @2x (Retina) transparently.
func computeScaleFromWindow(w *Window, tmpDir string) float64 {
	probe := filepath.Join(tmpDir, "scale-probe.png")
	if err := Capture(w, probe); err != nil {
		return 2.0
	}
	defer os.Remove(probe)
	return computeScale(w, probe)
}

func computeScale(w *Window, imagePath string) float64 {
	scale := 2.0
	if info, err := imageSize(imagePath); err == nil && w.Width > 0 {
		if s := float64(info.W) / float64(w.Width); s >= 1 {
			scale = s
		}
	}
	return scale
}

// imageSize returns the pixel dimensions of a PNG file. Duplicated here from
// cmd/findmy to keep the internal package self-contained.
func imageSize(path string) (struct{ W, H int }, error) {
	type dims struct{ W, H int }
	f, err := os.Open(path)
	if err != nil {
		return dims{}, err
	}
	defer f.Close()
	// Read PNG header: 8 magic bytes, then IHDR chunk (4 len + 4 type + 4 width + 4 height)
	var buf [24]byte
	if _, err := f.Read(buf[:]); err != nil {
		return dims{}, err
	}
	w := int(buf[16])<<24 | int(buf[17])<<16 | int(buf[18])<<8 | int(buf[19])
	h := int(buf[20])<<24 | int(buf[21])<<16 | int(buf[22])<<8 | int(buf[23])
	return dims{w, h}, nil
}

// parseDetailPane extracts the precise address from the FindMy detail panel.
// When the pin is clicked, FindMy shows a detail card on the right with:
//   - Person name/email
//   - Street address (e.g. "1 Rue de la Martinière, 91570 Bièvres")
//   - "Position actuelle" / "Current Location"
//   - Action buttons (Contacter, Itinéraire, etc.)
//
// Strategy: find the line just above "Position actuelle" (or its localized
// equivalent), which is the street address. Fallback: pick the line that
// looks most like an address (contains a postal code pattern).
func parseDetailPane(lines []TextLine, person *Person, sidebarRightPx int) string {
	personLower := strings.ToLower(person.Name)

	// Collect lines from the detail panel (right side, x > sidebarRightPx).
	type entry struct {
		text string
		y    int
	}
	var panel []entry

	for _, l := range lines {
		if l.X < sidebarRightPx {
			continue
		}
		txt := strings.TrimSpace(l.Text)
		if txt == "" {
			continue
		}
		panel = append(panel, entry{text: txt, y: l.Y})
	}

	sort.SliceStable(panel, func(i, j int) bool { return panel[i].y < panel[j].y })

	// Strategy 1: find the line just before "Position actuelle" / "Current Location".
	currentLocLabels := []string{
		"position actuelle", "current location", "posición actual",
		"aktuelle position", "posizione attuale", "posição atual",
		"現在地", "현재 위치", "当前位置", "текущая геопозиция",
	}
	for i, e := range panel {
		lower := strings.ToLower(e.text)
		for _, label := range currentLocLabels {
			// Must be the label itself (e.g. "Position actuelle"), not embedded
			// in another phrase (e.g. "Étiqueter la position actuelle").
			if lower == label || strings.HasPrefix(lower, label) {
				// The address is the line just above this label.
				if i > 0 {
					addr := panel[i-1].text
					// Skip if it's the person's name/email.
					if !strings.Contains(strings.ToLower(addr), personLower) &&
						!strings.Contains(addr, "@") && len(addr) > 5 {
						return addr
					}
				}
			}
		}
	}

	// Strategy 2: find a line that looks like a postal address.
	// Postal codes: FR "91570", DE "12345", US "CA 90210", etc.
	for _, e := range panel {
		if strings.Contains(strings.ToLower(e.text), personLower) || strings.Contains(e.text, "@") {
			continue
		}
		if looksLikeAddress(e.text) {
			return e.text
		}
	}

	return ""
}

// looksLikeAddress returns true if the text contains patterns typical of a
// street address: a postal code (digit sequence of 4-5), or a comma-separated
// city suffix.
func looksLikeAddress(s string) bool {
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		} else {
			if digits >= 4 && digits <= 5 {
				return true
			}
			digits = 0
		}
	}
	if digits >= 4 && digits <= 5 {
		return true
	}
	return strings.Contains(s, ",") && len(s) > 10
}

// --- Devices tab / Ring support ---

// Device represents an entry in the FindMy Devices sidebar.
type Device struct {
	Name     string `json:"name"`
	Location string `json:"location,omitempty"`
	Status   string `json:"status,omitempty"` // "En pause", "Maintenant", "Connecté(e)", etc.
	Group    string `json:"group,omitempty"`  // "Appareils de Christel", etc.
	NameY    int    `json:"-"`
	NameX    int    `json:"-"`
}

// PrepareDevices activates FindMy and switches to the Devices tab.
// Returns the window metadata. Caller should defer RestoreUserSpace() if needed.
func PrepareDevices() (*Window, error) {
	if err := requirePermissions(true); err != nil {
		return nil, err
	}

	wakeDisplay()

	w, _ := MainWindow()
	if w == nil || !w.OnScreen {
		previousApp = rememberFrontApp()
		if err := Activate(); err != nil {
			return nil, err
		}
		time.Sleep(900 * time.Millisecond)
	}

	_ = SwitchTab(GetAppStrings().DevicesTab)
	time.Sleep(1000 * time.Millisecond)

	return MainWindow()
}

// ScanDevices captures the Devices sidebar and parses device entries.
// It scrolls through the sidebar to find all devices, including those
// off-screen. Returns all found devices.
func ScanDevices(w *Window, tmpDir string) ([]Device, error) {
	// All coordinates derived from window geometry — never hardcoded.
	scale := computeScaleFromWindow(w, tmpDir)
	sidebarRightPx := int(340 * scale)
	textColMinPx := int(50 * scale)
	topMarginPx := int(90 * scale)

	var allDevices []Device
	seen := map[string]bool{}

	// Scroll target: center of the sidebar area (window-relative).
	sidebarX := w.X + int(170*scale/scale) // 170pt into the sidebar
	sidebarY := w.Y + w.Height/2

	emptyPasses := 0
	for scrollPass := 0; scrollPass < 15; scrollPass++ {
		shot := filepath.Join(tmpDir, fmt.Sprintf("devices_%d.png", scrollPass))
		if err := Capture(w, shot); err != nil {
			return allDevices, fmt.Errorf("capture devices: %w", err)
		}
		lines, err := OCR(shot)
		if err != nil {
			return allDevices, fmt.Errorf("ocr devices: %w", err)
		}
		_ = os.Remove(shot)

		devices := parseDeviceSidebar(lines, sidebarRightPx, textColMinPx, topMarginPx)
		newCount := 0
		for _, d := range devices {
			if !seen[d.Name] {
				seen[d.Name] = true
				allDevices = append(allDevices, d)
				newCount++
			}
		}

		if newCount == 0 {
			emptyPasses++
			if emptyPasses >= 3 {
				break // 3 consecutive empty passes → reached the end
			}
		} else {
			emptyPasses = 0
		}

		// Scroll down aggressively in the sidebar.
		_ = Scroll(sidebarX, sidebarY, -8)
		time.Sleep(500 * time.Millisecond)
	}

	return allDevices, nil
}

// parseDeviceSidebar extracts device entries from a Devices tab screenshot.
func parseDeviceSidebar(lines []TextLine, sidebarRightPx, textColMinPx, topMarginPx int) []Device {
	var rows []TextLine
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if l.X+l.Width/2 >= sidebarRightPx {
			continue
		}
		if l.Y < topMarginPx {
			continue
		}
		if l.X < textColMinPx {
			continue
		}
		rows = append(rows, l)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Y == rows[j].Y {
			return rows[i].X < rows[j].X
		}
		return rows[i].Y < rows[j].Y
	})

	skip := map[string]bool{
		"Personnes": true, "Appareils": true, "Objets": true,
		"People": true, "Devices": true, "Items": true,
		"Mentions légales": true, "Legal Notices": true,
	}

	var devices []Device
	var currentGroup string

	for _, l := range rows {
		txt := strings.TrimSpace(l.Text)
		if skip[txt] {
			continue
		}
		lower := strings.ToLower(txt)

		// Detect group headers: "Appareils de X"
		if strings.HasPrefix(lower, "appareils de ") || strings.HasPrefix(lower, "devices of ") ||
			strings.HasPrefix(lower, "geräte von ") || strings.HasPrefix(lower, "dispositivos de ") {
			currentGroup = txt
			continue
		}

		// Skip status/location lines (they follow a device name).
		if strings.Contains(lower, "en pause") || strings.Contains(lower, "aucune position") ||
			strings.Contains(lower, "connecté") || strings.Contains(lower, "maintenant") ||
			strings.Contains(lower, "pas de partage") || strings.Contains(lower, "position trouvée") ||
			strings.Contains(lower, "avec vous") || strings.Contains(lower, "ce mac") ||
			strings.Contains(lower, "rue ") || strings.Contains(lower, "no location") {
			// Attach to last device as status/location.
			if len(devices) > 0 {
				last := &devices[len(devices)-1]
				if last.Location == "" {
					last.Location = txt
				} else if last.Status == "" {
					last.Status = txt
				}
			}
			continue
		}

		// Distance info.
		if isDistance(txt) {
			continue
		}

		// Otherwise it's a device name.
		devices = append(devices, Device{
			Name:  txt,
			Group: currentGroup,
			NameY: l.Y,
			NameX: l.X,
		})
	}

	return devices
}

// FindDeviceByScroll scrolls through the Devices sidebar looking for a device
// whose name contains the target string. Returns the device with its current
// on-screen coordinates (ready for clicking). This is more reliable than a
// two-phase scan+scroll because coordinates are captured in the same position.
func FindDeviceByScroll(w *Window, target, tmpDir string) (*Device, error) {
	targetLower := strings.ToLower(target)
	scale := computeScaleFromWindow(w, tmpDir)
	sidebarRightPx := int(340 * scale)

	sidebarX := w.X + 170 // center of sidebar in screen points
	sidebarY := w.Y + w.Height/2

	// Scroll to top first.
	for i := 0; i < 20; i++ {
		_ = Scroll(sidebarX, sidebarY, 10)
	}
	time.Sleep(500 * time.Millisecond)

	// Scroll down, OCR each frame, look for the target.
	for pass := 0; pass < 20; pass++ {
		shot := filepath.Join(tmpDir, "scroll-find.png")
		if err := Capture(w, shot); err != nil {
			return nil, fmt.Errorf("capture: %w", err)
		}
		lines, err := OCR(shot)
		if err != nil {
			_ = os.Remove(shot)
			return nil, fmt.Errorf("ocr: %w", err)
		}
		_ = os.Remove(shot)

		for _, l := range lines {
			if l.X+l.Width/2 >= sidebarRightPx {
				continue // not in sidebar
			}
			txt := strings.TrimSpace(l.Text)
			if strings.Contains(strings.ToLower(txt), targetLower) {
				return &Device{
					Name:  txt,
					NameY: l.Y,
					NameX: l.X,
				}, nil
			}
		}

		_ = Scroll(sidebarX, sidebarY, -5)
		time.Sleep(400 * time.Millisecond)
	}

	return nil, fmt.Errorf("device %q not found after scrolling through entire list", target)
}

// playSoundLabels contains the localized "Play Sound" button text in all
// supported FindMy languages, used to locate the button in OCR results.
var playSoundLabels = []string{
	"émettre un son", "emettre un son", "play sound", "ton abspielen",
	"reproducir sonido", "riproduci suono", "emitir som", "sound abspielen",
	"サウンドを再生", "사운드 재생", "播放声音", "播放聲音",
}

// findPlaySoundButton scans OCR lines for the "Play Sound" button text.
// Returns the line if found, nil otherwise.
func findPlaySoundButton(lines []TextLine) *TextLine {
	for i, l := range lines {
		lower := strings.ToLower(strings.TrimSpace(l.Text))
		for _, label := range playSoundLabels {
			if strings.Contains(lower, label) {
				return &lines[i]
			}
		}
	}
	return nil
}

// findPinPopup looks for a popup bubble on the map (text outside the sidebar
// that resembles a device popup: mixed case, not a map label).
// Returns center coordinates in image pixels, or (0,0,false) if not found.
func findPinPopup(lines []TextLine, sidebarRightPx int) (cx, cy int, found bool) {
	var best TextLine
	bestRight := 0
	for _, l := range lines {
		if l.X < sidebarRightPx {
			continue
		}
		txt := strings.TrimSpace(l.Text)
		if len(txt) < 4 || txt == "3D" || txt == "N" || txt == "+" {
			continue
		}
		// Skip all-caps map labels (RUE LÉON MIGNOTTE, etc.).
		if txt == strings.ToUpper(txt) && len(txt) > 5 {
			continue
		}
		right := l.X + l.Width
		if right > bestRight {
			bestRight = right
			best = l
		}
	}
	if bestRight == 0 {
		return 0, 0, false
	}
	return best.X + best.Width/2, best.Y + best.Height/2, true
}

// pollOCR repeatedly captures+OCRs until match returns true or timeout.
// Returns the matched OCR lines on success, nil on timeout.
func pollOCR(w *Window, tmpDir string, timeout time.Duration, interval time.Duration, match func([]TextLine) bool) ([]TextLine, error) {
	deadline := time.Now().Add(timeout)
	shot := filepath.Join(tmpDir, "poll.png")
	for {
		if err := Capture(w, shot); err != nil {
			return nil, err
		}
		lines, err := OCR(shot)
		if err != nil {
			return nil, err
		}
		_ = os.Remove(shot)
		if match(lines) {
			return lines, nil
		}
		if time.Now().After(deadline) {
			return nil, nil // timeout, no match
		}
		time.Sleep(interval)
	}
}

// RingDevice opens a device's detail card on the map and locates the
// "Play Sound" button. Hybrid: fast-path if already open, map-center
// fallback (proven 95% reliable), poll-based detection for speed.
//
// Set dryRun=true to perform all steps except the final click (for testing).
func RingDevice(w *Window, device *Device, tmpDir string, dryRun bool) error {
	if err := requirePermissions(true); err != nil {
		return err
	}

	// Activate FindMy (needed for Catalyst clicks to register).
	_ = Activate()
	frontScript := `tell application "System Events" to tell process "FindMy" to set frontmost to true`
	_ = exec.Command("osascript", "-e", frontScript).Run()
	time.Sleep(400 * time.Millisecond)
	defer RestoreUserSpace()

	scale := computeScaleFromWindow(w, tmpDir)

	// Fast-path: detail card already open from previous run?
	if lines, err := captureAndOCR(w, tmpDir); err == nil {
		if btn := findPlaySoundButton(lines); btn != nil {
			return clickOrDryRun(w, btn, scale, dryRun)
		}
	}

	// 1. Click the device in the sidebar.
	screenX := w.X + int(float64(device.NameX)/scale)
	screenY := w.Y + int(float64(device.NameY)/scale) + 8
	if err := Click(screenX, screenY); err != nil {
		return fmt.Errorf("click device: %w", err)
	}

	// 2. Wait for the map to zoom (fixed 3s — empirically the most reliable).
	time.Sleep(3 * time.Second)

	// 3. Double-click map center → popup → detail card.
	//    Poll OCR after each attempt for early exit instead of fixed sleep.
	sidebarPts := 340
	mapCenterX := w.X + sidebarPts + (w.Width-sidebarPts)/2
	mapCenterY := w.Y + w.Height/2

	var buttonLine *TextLine
	for attempt := 0; attempt < 5; attempt++ {
		_ = Click(mapCenterX, mapCenterY)
		time.Sleep(time.Duration(1000+attempt*200) * time.Millisecond)
		_ = Click(mapCenterX, mapCenterY)

		// Poll for the button to appear (early exit instead of fixed sleep).
		lines, _ := pollOCR(w, tmpDir,
			time.Duration(2000+attempt*500)*time.Millisecond,
			300*time.Millisecond,
			func(lines []TextLine) bool {
				return findPlaySoundButton(lines) != nil
			})

		if lines != nil {
			buttonLine = findPlaySoundButton(lines)
			break
		}
	}

	if buttonLine == nil {
		return fmt.Errorf("'Play Sound' button not found (device may be offline or pin not clickable)")
	}

	return clickOrDryRun(w, buttonLine, scale, dryRun)
}

// captureAndOCR is a one-shot helper: take a screenshot, OCR it, clean up.
func captureAndOCR(w *Window, tmpDir string) ([]TextLine, error) {
	shot := filepath.Join(tmpDir, "snap.png")
	if err := Capture(w, shot); err != nil {
		return nil, err
	}
	defer os.Remove(shot)
	return OCR(shot)
}

// clickOrDryRun centers a click on the button's text line, or prints the
// target coordinates in dry-run mode.
func clickOrDryRun(w *Window, btn *TextLine, scale float64, dryRun bool) error {
	// Click ~25px above the text — the actual icon sits above the label.
	btnX := w.X + int(float64(btn.X+btn.Width/2)/scale)
	btnY := w.Y + int(float64(btn.Y)/scale) - int(20/scale)
	if dryRun {
		fmt.Fprintf(os.Stderr, "dry-run: would click 'Play Sound' at screen (%d, %d)\n", btnX, btnY)
		return nil
	}
	if err := Click(btnX, btnY); err != nil {
		return fmt.Errorf("click play sound: %w", err)
	}
	return nil
}
