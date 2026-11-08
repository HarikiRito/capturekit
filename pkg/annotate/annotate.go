// Package annotate draws colored boxes and labels on screenshots using ffmpeg.
package annotate

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultFont  = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
	maxLabelLen  = 200
	maxThickness = 100
)

const defaultThickness = 8

type boxSpec struct {
	Box      []int  `json:"box"`
	Label    string `json:"label"`
	LabelPos string `json:"labelPos"`
	Color    string `json:"color"`
}

// job's own box/label/labelPos/color fields are the single-box form and the defaults for Boxes.
type job struct {
	boxSpec
	Image     string    `json:"image"`
	Out       string    `json:"out"`
	Thickness int       `json:"thickness"`
	Boxes     []boxSpec `json:"boxes"`
}

// specs returns the boxes to draw, with job-level labelPos and color filled in.
func (j job) specs() []boxSpec {
	list := j.Boxes
	if len(list) == 0 {
		if j.Box == nil && j.Label == "" {
			return nil
		}
		list = []boxSpec{j.boxSpec}
	}
	out := make([]boxSpec, len(list))
	for i, b := range list {
		if b.LabelPos == "" {
			b.LabelPos = j.LabelPos
		}
		if b.Color == "" {
			b.Color = j.Color
		}
		out[i] = b
	}
	return out
}

// Run executes the annotate command and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "check" {
		return check(stdout, stderr)
	}
	fs := flag.NewFlagSet("annotate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	in := fs.String("in", "", "input image (png/jpeg)")
	box := fs.String("box", "", "x,y,w,h in pixels (optional)")
	label := fs.String("label", "", "label text (optional)")
	out := fs.String("out", "", "output image")
	manifest := fs.String("manifest", "", "batch manifest json")
	baseDir := fs.String("base-dir", "", "base dir for manifest paths (default: manifest dir)")
	labelPos := fs.String("label-pos", "", "label placement: "+strings.Join(labelPositions, "|")+" (default auto)")
	thickness := fs.Int("thickness", defaultThickness, "box line thickness in pixels")
	color := fs.String("color", "", "box and label color: ffmpeg name or #rrggbb (default red)")
	font := fs.String("font", defaultFont, "ttf font file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var jobs []job
	if *manifest != "" {
		if *in != "" || *out != "" {
			fmt.Fprintln(stderr, "error: --manifest excludes --in/--out")
			return 2
		}
		base := *baseDir
		if base == "" {
			base = filepath.Dir(*manifest)
		}
		var err error
		if jobs, err = loadManifest(*manifest, base); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 1
		}
	} else {
		if *in == "" || *out == "" {
			fmt.Fprintln(stderr, "error: need --in and --out, or --manifest")
			return 2
		}
		j := job{Image: *in, Out: *out, Thickness: *thickness}
		j.Label, j.LabelPos, j.Color = *label, *labelPos, *color
		if *box != "" {
			var err error
			if j.Box, err = parseBox(*box); err != nil {
				fmt.Fprintln(stderr, "error:", err)
				return 2
			}
		}
		jobs = []job{j}
	}

	if errs := validateAll(jobs); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(stderr, "error:", e)
		}
		return 1
	}
	return runAll(jobs, *font, stdout, stderr)
}

func runAll(jobs []job, font string, stdout, stderr io.Writer) int {
	if _, err := os.Stat(font); err != nil {
		fmt.Fprintf(stderr, "error: font not found: %s\n", font)
		return 1
	}
	code := 0
	for _, j := range jobs {
		if err := render(j, font); err != nil {
			fmt.Fprintf(stderr, "error: %s: %v\n", j.Out, err)
			code = 1
			continue
		}
		fmt.Fprintf(stdout, "ok %s\n", j.Out)
	}
	return code
}

func check(stdout, stderr io.Writer) int {
	code := 0
	if p, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Fprintln(stderr, "missing: ffmpeg not on PATH")
		code = 1
	} else {
		fmt.Fprintln(stdout, "ok ffmpeg", p)
	}
	if _, err := os.Stat(defaultFont); err != nil {
		fmt.Fprintln(stderr, "missing: font", defaultFont, "(pass --font to override)")
		code = 1
	} else {
		fmt.Fprintln(stdout, "ok font", defaultFont)
	}
	return code
}

func parseBox(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	box := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("--box must be integers x,y,w,h: %q", s)
		}
		box = append(box, n)
	}
	return box, nil
}

func render(j job, font string) error {
	if err := os.MkdirAll(filepath.Dir(j.Out), 0o755); err != nil {
		return err
	}
	if len(j.specs()) == 0 {
		return copyFile(j.Image, j.Out)
	}
	w, h, err := imageSize(j.Image)
	if err != nil {
		return err
	}
	cmd := exec.Command("ffmpeg", "-loglevel", "error", "-y", "-i", j.Image, "-vf", filter(j, w, h, font), "-frames:v", "1", j.Out)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
