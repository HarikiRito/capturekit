package annotate

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for image.DecodeConfig
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

func loadManifest(path, base string) ([]job, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var jobs []job
	if err := json.Unmarshal(b, &jobs); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", path, err)
	}
	for i := range jobs {
		jobs[i].Image = joinBase(base, jobs[i].Image)
		jobs[i].Out = joinBase(base, jobs[i].Out)
	}
	return jobs, nil
}

func joinBase(base, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

func imageSize(path string) (w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: not a png/jpeg: %w", path, err)
	}
	return cfg.Width, cfg.Height, nil
}

// validateAll returns every problem across all jobs.
func validateAll(jobs []job) []string {
	var errs []string
	if len(jobs) == 0 {
		return []string{"no jobs in manifest"}
	}
	for i, j := range jobs {
		for _, e := range validate(j) {
			errs = append(errs, fmt.Sprintf("job %d (%s): %s", i+1, j.Out, e))
		}
	}
	return errs
}

var (
	hexColor   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	namedColor = map[string]bool{"red": true, "green": true, "blue": true, "yellow": true, "orange": true, "magenta": true,
		"cyan": true, "purple": true, "pink": true, "black": true, "white": true, "lime": true, "navy": true, "teal": true}
)

// colorArg returns the ffmpeg form of a validated color; empty means red.
func colorArg(c string) string {
	switch {
	case c == "":
		return "red"
	case hexColor.MatchString(c):
		return "0x" + c[1:]
	}
	return strings.ToLower(c)
}

func validColor(c string) bool {
	return c == "" || hexColor.MatchString(c) || namedColor[strings.ToLower(c)]
}

func validate(j job) []string {
	var errs []string
	if j.Out == "" {
		errs = append(errs, "out is required")
	}
	if j.Image == "" {
		return append(errs, "image is required")
	}
	w, h, err := imageSize(j.Image)
	if err != nil {
		return append(errs, err.Error())
	}
	if j.Thickness < 0 || j.Thickness > maxThickness {
		errs = append(errs, fmt.Sprintf("thickness must be 1-%d", maxThickness))
	}
	if len(j.Boxes) > 0 && (j.Box != nil || j.Label != "") {
		errs = append(errs, "boxes excludes box/label; put them inside boxes")
	}
	specs := j.specs()
	for i, b := range specs {
		prefix := ""
		if len(j.Boxes) > 0 {
			prefix = fmt.Sprintf("boxes[%d]: ", i)
		}
		for _, e := range validateSpec(b, w, h) {
			errs = append(errs, prefix+e)
		}
	}
	return errs
}

func validateSpec(b boxSpec, w, h int) []string {
	var errs []string
	if n := utf8.RuneCountInString(b.Label); n > maxLabelLen {
		errs = append(errs, fmt.Sprintf("label is %d chars, max %d", n, maxLabelLen))
	}
	if b.LabelPos != "" && !slices.Contains(labelPositions, b.LabelPos) {
		errs = append(errs, fmt.Sprintf("labelPos %q must be one of %s", b.LabelPos, strings.Join(labelPositions, ", ")))
	}
	if !validColor(b.Color) {
		errs = append(errs, fmt.Sprintf("color %q must be a named color (e.g. red, blue) or #rrggbb", b.Color))
	}
	if b.Box == nil {
		return errs
	}
	if len(b.Box) != 4 {
		return append(errs, "box must be [x,y,w,h]")
	}
	x, y, bw, bh := b.Box[0], b.Box[1], b.Box[2], b.Box[3]
	if bw <= 0 || bh <= 0 {
		errs = append(errs, "box w,h must be > 0")
	}
	if x < 0 || y < 0 || x+bw > w || y+bh > h {
		errs = append(errs, fmt.Sprintf("box [%d,%d,%d,%d] outside image %dx%d", x, y, bw, bh, w, h))
	}
	return errs
}
