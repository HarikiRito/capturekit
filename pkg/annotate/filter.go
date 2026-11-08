package annotate

import (
	"fmt"
	"strconv"
	"strings"
)

var labelPositions = []string{"auto", "above", "below", "inside-top", "inside-bottom", "left", "right"}

// filter builds the ffmpeg -vf chain: a drawbox and optional drawtext per box.
func filter(j job, imgW, imgH int, font string) string {
	var parts []string
	fs := max(imgW/53, 24)
	th := j.Thickness
	if th == 0 {
		th = defaultThickness
	}
	for _, b := range j.specs() {
		col := colorArg(b.Color)
		if b.Box == nil {
			parts = append(parts, drawtext(b.Label, font, col, fs, clampX("0", fs), fs))
			continue
		}
		parts = append(parts, fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=%s:t=%d", b.Box[0], b.Box[1], b.Box[2], b.Box[3], col, th))
		if b.Label != "" {
			x, y := labelPlace(b.LabelPos, b.Box, th, imgW, imgH, fs)
			parts = append(parts, drawtext(b.Label, font, col, fs, x, y))
		}
	}
	return strings.Join(parts, ",")
}

// labelPlace returns the drawtext x expression and y for a label next to box; both are clamped inside the image.
func labelPlace(pos string, box []int, th, imgW, imgH, fs int) (x string, y int) {
	bx, by, bw, bh := box[0], box[1], box[2], box[3]
	pad := fs / 3
	if pos == "" || pos == "auto" {
		pos = "above"
		if by < 3*fs {
			pos = "below"
		}
	}
	x = strconv.Itoa(bx)
	switch pos {
	case "above":
		y = by - 2*fs
	case "below":
		y = by + bh + fs*5/8
	case "inside-top":
		x, y = strconv.Itoa(bx+th+pad), by+th+pad
	case "inside-bottom":
		x, y = strconv.Itoa(bx+th+pad), by+bh-th-pad-fs
	case "left":
		x, y = fmt.Sprintf("%d-text_w-%d", bx, pad*2), by
	case "right":
		x, y = strconv.Itoa(bx+bw+pad*2), by
	}
	y = max(min(y, imgH-fs-pad), fs/2)
	return clampX(x, fs), y
}

// clampX keeps the label's left edge and right edge at least a margin inside the image.
func clampX(x string, fs int) string {
	margin := fs * 5 / 6
	return fmt.Sprintf("min(max(%s\\,%d)\\,w-text_w-%d)", x, margin, margin)
}

func drawtext(label, font, color string, fs int, x string, y int) string {
	return fmt.Sprintf("drawtext=fontfile=%s:text=%s:fontsize=%d:fontcolor=white:box=1:boxcolor=%s@0.95:boxborderw=%d:x=%s:y=%d",
		escapeFilter(font), escapeText(label), fs, color, fs/3, x, y)
}

// escapeText escapes for drawtext's text option, then for the filtergraph.
func escapeText(s string) string {
	return escapeFilter(escapeDrawtext(s))
}

func escapeDrawtext(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`, `%`, `\\%`).Replace(s)
}

func escapeFilter(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, `,`, `\,`, `;`, `\;`, `[`, `\[`, `]`, `\]`).Replace(s)
}
