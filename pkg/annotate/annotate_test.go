package annotate

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestManifestReportsAllErrors(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), 100, 100)
	m := `[
	 {"image":"a.png","box":[90,10,50,20],"label":"x","out":"o1.png"},
	 {"image":"missing.png","out":"o2.png"},
	 {"image":"a.png","box":[1,2,0,5],"out":"o3.png"},
	 {"image":"a.png","box":[1,2,3],"out":"o4.png"}]`
	mp := filepath.Join(dir, "m.json")
	if err := os.WriteFile(mp, []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	if code := Run([]string{"--manifest", mp}, &out, &errb); code != 1 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{"job 1", "outside image", "job 2", "job 3", "w,h must be > 0", "job 4", "[x,y,w,h]"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, errb.String())
		}
	}
	if out.Len() != 0 {
		t.Errorf("rendered despite errors: %s", out.String())
	}
}

func TestEscapeText(t *testing.T) {
	got := escapeText(`Don't: 50%`)
	want := `Don\\\'t\\: 50\\\\%`
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestRunWritesAnnotatedImage(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg missing")
	}
	if _, err := os.Stat(defaultFont); err != nil {
		t.Skip("font missing")
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.png"), filepath.Join(dir, "sub", "out.png")
	writePNG(t, in, 640, 480)

	var so, se bytes.Buffer
	args := []string{"--in", in, "--box", "100,200,200,100", "--label", "Don't: tap 50%", "--out", out}
	if code := Run(args, &so, &se); code != 0 {
		t.Fatalf("code %d: %s", code, se.String())
	}
	if so.String() != "ok "+out+"\n" {
		t.Errorf("stdout %q", so.String())
	}
	w, h, err := imageSize(out)
	if err != nil || w != 640 || h != 480 {
		t.Fatalf("size %dx%d err %v", w, h, err)
	}
	a, _ := os.ReadFile(in)
	b, _ := os.ReadFile(out)
	if bytes.Equal(a, b) {
		t.Error("output identical to input")
	}

	boxOnly := filepath.Join(dir, "box.png")
	Run([]string{"--in", in, "--box", "100,200,200,100", "--out", boxOnly}, &so, &se)
	c, _ := os.ReadFile(boxOnly)
	if bytes.Equal(b, c) {
		t.Error("label not drawn")
	}
}

func TestLabelPlace(t *testing.T) {
	const fs, th = 24, 8 // pad 8, margin 20
	box := []int{200, 300, 100, 60}
	cases := []struct {
		pos   string
		wantX string
		wantY int
	}{
		{"", "200", 300 - 2*fs},
		{"above", "200", 252},
		{"below", "200", 300 + 60 + 15},
		{"inside-top", "216", 316},
		{"inside-bottom", "216", 300 + 60 - 8 - 8 - fs},
		{"left", "200-text_w-16", 300},
		{"right", "316", 300},
	}
	for _, c := range cases {
		x, y := labelPlace(c.pos, box, th, 1000, 800, fs)
		if want := clampX(c.wantX, fs); x != want || y != c.wantY {
			t.Errorf("%q: got %s,%d want %s,%d", c.pos, x, y, want, c.wantY)
		}
	}
}

func TestLabelPlaceAutoFlipsBelowNearTop(t *testing.T) {
	_, y := labelPlace("auto", []int{10, 20, 100, 50}, 8, 1000, 800, 24)
	if y != 20+50+15 {
		t.Errorf("y=%d", y)
	}
}

func TestLabelPlaceClampsY(t *testing.T) {
	const fs = 24
	if _, y := labelPlace("above", []int{10, 5, 50, 50}, 8, 1000, 800, fs); y != fs/2 {
		t.Errorf("top clamp y=%d", y)
	}
	if _, y := labelPlace("below", []int{10, 700, 50, 90}, 8, 1000, 800, fs); y != 800-fs-fs/3 {
		t.Errorf("bottom clamp y=%d", y)
	}
	x, _ := labelPlace("right", []int{900, 100, 90, 50}, 8, 1000, 800, fs)
	if !strings.HasPrefix(x, "min(max(") || !strings.HasSuffix(x, "w-text_w-20)") {
		t.Errorf("x not clamped: %s", x)
	}
}

func TestManifestValidatesNewFields(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "a.png"), 100, 100)
	m := `[
	 {"image":"a.png","box":[1,1,5,5],"labelPos":"middle","out":"o1.png"},
	 {"image":"a.png","box":[1,1,5,5],"color":"notacolor","thickness":-2,"out":"o2.png"},
	 {"image":"a.png","box":[1,1,5,5],"boxes":[{"box":[1,1,5,5]}],"out":"o3.png"},
	 {"image":"a.png","out":"o4.png","boxes":[{"box":[1,1,5,5],"labelPos":"left","color":"#00ff00"},{"box":[99,1,5,5],"color":"zz"}]}]`
	mp := filepath.Join(dir, "m.json")
	if err := os.WriteFile(mp, []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Run([]string{"--manifest", mp}, &out, &errb); code != 1 {
		t.Fatalf("code %d", code)
	}
	for _, want := range []string{`labelPos "middle"`, `color "notacolor"`, "thickness must be", "boxes excludes", "job 4", "boxes[1]: color", "boxes[1]: box [99"} {
		if !strings.Contains(errb.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, errb.String())
		}
	}
	if strings.Contains(errb.String(), "boxes[0]") {
		t.Errorf("valid box flagged:\n%s", errb.String())
	}
}

func pixel(t *testing.T, path string, x, y int) color.RGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, _ := img.At(x, y).RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
}

func TestRunDrawsMultipleColoredBoxes(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg missing")
	}
	if _, err := os.Stat(defaultFont); err != nil {
		t.Skip("font missing")
	}
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "in.png"), 640, 480)
	m := `[{"image":"in.png","out":"out.png","thickness":6,"boxes":[
	  {"box":[40,150,200,100],"label":"A","labelPos":"inside-top","color":"blue"},
	  {"box":[400,150,200,100],"label":"B","labelPos":"left","color":"#00ff00"}]}]`
	mp := filepath.Join(dir, "m.json")
	if err := os.WriteFile(mp, []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	var so, se bytes.Buffer
	if code := Run([]string{"--manifest", mp}, &so, &se); code != 0 {
		t.Fatalf("code %d: %s", code, se.String())
	}
	out := filepath.Join(dir, "out.png")
	if got := pixel(t, out, 42, 200); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("blue box edge: %v", got)
	}
	if got := pixel(t, out, 402, 200); got != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("green box edge: %v", got)
	}
	if got := pixel(t, out, 300, 400); got != (color.RGBA{200, 200, 200, 255}) {
		t.Errorf("background touched: %v", got)
	}
}
