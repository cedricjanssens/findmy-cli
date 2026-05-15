package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oshahine/findmy-cli/internal/findmy"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "people":
		runPeople(os.Args[2:])
	case "person":
		runPerson(os.Args[2:])
	case "devices":
		runDevices(os.Args[2:])
	case "ring":
		runRing(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `findmy — query Apple's Find My from the command line via UI scraping.

USAGE
  findmy people [--json] [--keep]
  findmy person <name> [--json] [--zoom] [--keep]
  findmy devices [--json] [--keep]
  findmy ring <device> [--confirm] [--keep]

FLAGS
  --json      output JSON instead of a human-readable table
  --zoom      click to fetch the precise street address (person only)
  --confirm   required for ring — actually plays the sound (default: dry-run)
  --keep      preserve debug screenshots in /tmp/findmy-cli/

RECOMMENDED SETUP
  Run "findmy-helper setup-check" to verify your environment.

  REQUIRED — TCC permissions
    Screen Recording  (for screencapture)
    Accessibility     (for synthesizing clicks / scrolls)
    Grant in System Settings → Privacy & Security, then fully restart
    the host process (TCC is read once at process start).

  OPTIMAL — Virtual display (zero visual disruption)
    The basic commands (people, person without --zoom, devices listing)
    only need to capture FindMy's window. If FindMy runs on a secondary
    display (a BetterDisplay virtual screen, or a hardware HDMI/USB-C
    dummy plug), captures happen invisibly — no flicker, no Space
    switching, no focus theft.
      brew install --cask betterdisplay
    Then move FindMy.app to the virtual display via the Window menu.

  OPTIMAL — Dedicated user session for ring / --zoom
    The "ring" command and the "--zoom" mode need to click inside
    FindMy. CGEvent clicks move the system cursor and steal keyboard
    focus, which interrupts whatever you are doing on your main
    session (typing, gaming, etc.). Running findmy-cli from a
    dedicated macOS user account (Fast User Switching) and exposing
    it remotely (HTTP/MCP over Tailscale, or plain SSH) keeps these
    interactions isolated from your primary workspace.

EXAMPLES
  findmy people --json
  findmy person "cedric.janssens@gmail.com" --zoom --json
  findmy devices
  findmy ring "iPhone14PM Christel"            # dry-run, locates button
  findmy ring "iPhone14PM Christel" --confirm  # actually rings the device`)
	os.Exit(2)
}

type runOpts struct {
	json    bool
	keep    bool
	zoom    bool
	confirm bool
}

// parseOpts splits args into known flags and positional args. Go's flag
// package stops at the first non-flag, but `findmy person Omar Shahine
// --json` puts the flag after positional args, so we pre-extract flags
// from anywhere in the slice.
func parseOpts(args []string) (runOpts, []string) {
	var o runOpts
	var positional []string
	for _, a := range args {
		switch a {
		case "--json", "-json":
			o.json = true
		case "--keep", "-keep":
			o.keep = true
		case "--zoom", "-zoom":
			o.zoom = true
		case "--confirm", "-confirm":
			o.confirm = true
		default:
			positional = append(positional, a)
		}
	}
	_ = flag.CommandLine
	return o, positional
}

func tmpDir() string {
	d := "/tmp/findmy-cli"
	_ = os.MkdirAll(d, 0o755)
	return d
}

func runPeople(args []string) {
	opts, _ := parseOpts(args)

	w, err := findmy.PreparePeople()
	must(err)
	shot := filepath.Join(tmpDir(), "people.png")
	must(findmy.Capture(w, shot))
	findmy.RestoreUserSpace() // switch back after capture
	defer cleanup(shot, opts.keep)

	lines, err := findmy.OCR(shot)
	must(err)

	sidebarRightPx, textColMinPx, topMarginPx := pixelLayout(w, shot)
	people := findmy.ParsePeople(lines, sidebarRightPx, textColMinPx, topMarginPx)

	if opts.json {
		emitJSON(people)
		return
	}
	if len(people) == 0 {
		fmt.Println("(no people found)")
		return
	}
	sort.SliceStable(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	for _, p := range people {
		fmt.Printf("%s\n  %s", p.Name, p.Location)
		if p.Staleness != "" {
			fmt.Printf("  (%s)", p.Staleness)
		}
		if p.Distance != "" {
			fmt.Printf("  [%s]", p.Distance)
		}
		fmt.Println()
	}
}

func runPerson(args []string) {
	opts, rest := parseOpts(args)
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: findmy person <name> [--json] [--zoom]")
		os.Exit(2)
	}
	target := strings.ToLower(strings.Join(rest, " "))

	w, err := findmy.PreparePeople()
	must(err)
	shot := filepath.Join(tmpDir(), "people.png")
	must(findmy.Capture(w, shot))
	if !opts.zoom {
		findmy.RestoreUserSpace() // switch back early if no zoom needed
	}
	defer cleanup(shot, opts.keep)

	lines, err := findmy.OCR(shot)
	must(err)

	sidebarRightPx, textColMinPx, topMarginPx := pixelLayout(w, shot)
	people := findmy.ParsePeople(lines, sidebarRightPx, textColMinPx, topMarginPx)

	var match *findmy.Person
	for i := range people {
		if strings.EqualFold(strings.TrimSpace(people[i].Name), target) {
			match = &people[i]
			break
		}
	}
	if match == nil {
		for i := range people {
			if strings.Contains(strings.ToLower(people[i].Name), target) {
				match = &people[i]
				break
			}
		}
	}
	if match == nil {
		fmt.Fprintf(os.Stderr, "no person matching %q in sidebar\n", target)
		os.Exit(1)
	}

	if opts.zoom {
		detailShot := filepath.Join(tmpDir(), "detail.png")
		defer cleanup(detailShot, opts.keep)
		addr, err := findmy.DetailAddress(w, match, shot, detailShot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: zoom failed: %v\n", err)
		} else if addr != "" {
			match.Address = addr
		}
	}

	if opts.json {
		emitJSON(match)
		return
	}
	fmt.Printf("%s\n  %s", match.Name, match.Location)
	if match.Address != "" {
		fmt.Printf("\n  %s", match.Address)
	}
	if match.Staleness != "" {
		fmt.Printf("  (%s)", match.Staleness)
	}
	if match.Distance != "" {
		fmt.Printf("  [%s]", match.Distance)
	}
	fmt.Println()
}


// pixelLayout returns the sidebar-right and name-column-left thresholds in
// image pixels. The FindMy sidebar is ~340pt wide; the avatar column is
// ~100pt with the avatar circle centered around 50pt, so an 80pt cutoff
// drops centered avatar OCR fragments while admitting real name/location
// text that begins around 90pt. We use a float scale because some displays
// (e.g. a 4K dummy plug) report non-integer pixel-per-point ratios.
func pixelLayout(w *findmy.Window, imagePath string) (sidebarRightPx, textColMinPx, topMarginPx int) {
	scale := imageScale(w, imagePath)
	return int(340 * scale), int(80 * scale), int(120 * scale)
}

func windowPointFromImagePoint(w *findmy.Window, imagePath string, px, py int) (int, int) {
	scale := imageScale(w, imagePath)
	return w.X + int(float64(px)/scale), w.Y + int(float64(py)/scale)
}

func imageScale(w *findmy.Window, imagePath string) float64 {
	scale := 2.0
	if info, err := imageSize(imagePath); err == nil && w.Width > 0 {
		if s := float64(info.W) / float64(w.Width); s >= 1 {
			scale = s
		}
	}
	return scale
}

type imgInfo struct{ W, H int }

func imageSize(path string) (imgInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return imgInfo{}, err
	}
	defer f.Close()
	cfg, _, err := decodeConfig(f)
	if err != nil {
		return imgInfo{}, err
	}
	return imgInfo{W: cfg.Width, H: cfg.Height}, nil
}

func runDevices(args []string) {
	opts, _ := parseOpts(args)

	w, err := findmy.PrepareDevices()
	must(err)
	defer findmy.RestoreUserSpace()

	devices, err := findmy.ScanDevices(w, tmpDir())
	must(err)

	// Switch back to People tab for next run.
	_ = findmy.SwitchTab(findmy.GetAppStrings().PeopleTab)

	if opts.json {
		emitJSON(devices)
		return
	}
	if len(devices) == 0 {
		fmt.Println("(no devices found)")
		return
	}
	for _, d := range devices {
		fmt.Printf("%s", d.Name)
		if d.Location != "" {
			fmt.Printf("  %s", d.Location)
		}
		if d.Group != "" {
			fmt.Printf("  [%s]", d.Group)
		}
		fmt.Println()
	}
}

func runRing(args []string) {
	opts, rest := parseOpts(args)
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "usage: findmy ring <device> [--confirm]")
		os.Exit(2)
	}
	target := strings.ToLower(strings.Join(rest, " "))

	if !opts.confirm {
		fmt.Fprintf(os.Stderr, "This will make the device play a loud sound.\nAdd --confirm to actually ring it.\n")
		fmt.Fprintf(os.Stderr, "Running in dry-run mode (will locate the button but not click it).\n\n")
	}

	w, err := findmy.PrepareDevices()
	must(err)

	// Single-pass: scroll through the sidebar looking for the target device.
	// No pre-scan — just scroll and OCR until found or exhausted.
	match, err := findmy.FindDeviceByScroll(w, target, tmpDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Found: %s\n", match.Name)

	dryRun := !opts.confirm
	if err := findmy.RingDevice(w, match, tmpDir(), dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if dryRun {
		fmt.Println("Dry-run complete. Add --confirm to ring.")
	} else {
		fmt.Printf("Ringing %s...\n", match.Name)
	}

	// Restore People tab.
	_ = findmy.SwitchTab(findmy.GetAppStrings().PeopleTab)
	findmy.RestoreUserSpace()
}

func emitJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func cleanup(path string, keep bool) {
	if !keep {
		_ = os.Remove(path)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
