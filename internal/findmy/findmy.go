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

// PreparePeople activates FindMy, selects the People tab, then switches back
// to the user's original app/Space. screencapture -l works across Spaces so
// the capture can happen after the switch-back. Returns the window metadata.
func PreparePeople() (*Window, error) {
	if err := requirePermissions(false); err != nil {
		return nil, err
	}

	wakeDisplay()
	hintDedicatedSpace()
	previousApp := rememberFrontApp()

	if err := Activate(); err != nil {
		return nil, err
	}
	time.Sleep(900 * time.Millisecond)
	frontScript := `tell application "System Events" to tell process "FindMy" to set frontmost to true`
	_ = exec.Command("osascript", "-e", frontScript).Run()
	_ = SwitchTab(GetAppStrings().PeopleTab)
	time.Sleep(500 * time.Millisecond)

	// Switch back to the user's Space immediately. The People tab is already
	// painted and screencapture -l will capture it across Spaces.
	restoreFrontApp(previousApp)
	time.Sleep(800 * time.Millisecond)

	return MainWindow()
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
func ParsePeople(lines []TextLine, sidebarRightPx, textColMinPx int) []Person {
	rows := make([]TextLine, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		if l.X+l.Width/2 >= sidebarRightPx {
			continue
		}
		if l.Y < 240 {
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

	// --zoom requires clicks → must be on FindMy's Space.
	previousApp := rememberFrontApp()
	_ = Activate()
	time.Sleep(500 * time.Millisecond)
	defer restoreFrontApp(previousApp)

	scale := computeScale(w, sidebarShot)
	sidebarRightPx := int(340 * scale)

	// Click the person's name row in the sidebar to zoom the map.
	screenX := w.X + int(float64(person.NameX)/scale) + 20
	screenY := w.Y + int(float64(person.NameY)/scale) + 8
	if err := Click(screenX, screenY); err != nil {
		return "", fmt.Errorf("click person row: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Click the pin on the map to open the detail card. The pin is roughly
	// centered in the map area after the sidebar click zooms to the person.
	// We retry up to 3 times, checking OCR for the detail card after each attempt.
	mapCenterX := w.X + int(float64(sidebarRightPx)/scale) + (w.Width-int(float64(sidebarRightPx)/scale))/2
	mapCenterY := w.Y + w.Height/2

	for attempt := 0; attempt < 3; attempt++ {
		_ = Click(mapCenterX, mapCenterY)
		time.Sleep(1500 * time.Millisecond)

		if err := Capture(w, detailDest); err != nil {
			return "", fmt.Errorf("detail capture: %w", err)
		}
		lines, err := OCR(detailDest)
		if err != nil {
			return "", fmt.Errorf("detail ocr: %w", err)
		}
		if addr := parseDetailPane(lines, person, sidebarRightPx); addr != "" {
			return addr, nil
		}
	}

	return "", fmt.Errorf("detail card did not appear after 3 click attempts")
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
			if strings.Contains(lower, label) {
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
