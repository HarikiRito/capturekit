package annotate

// Instructions returns the agent-readable command reference.
func Instructions() string {
	return `capture annotate - draw colored boxes + white-on-color labels on screenshots (needs ffmpeg)

commands
  capture annotate check
      verify ffmpeg and the font exist
  capture annotate --in IN.png --box x,y,w,h --label "text" --out OUT.png [--label-pos POS] [--thickness N] [--color C] [--font TTF]
      annotate one image. --box and --label are optional:
      box+label = box with label above (below if box is near the top)
      box only (empty label) = box only; no box and no label = plain copy
  capture annotate --manifest m.json [--base-dir DIR] [--font TTF]
      batch mode. paths in the manifest are relative to --base-dir
      (default: the manifest's directory); output dirs are created

label placement (--label-pos / "labelPos"), use it when the default label would cover nearby text
  auto (default) above if room else below | above | below
  inside-top, inside-bottom: inside the box at its left corner
  left, right: beside the box, top-aligned
  labels are always clamped to stay inside the image.
--thickness N / "thickness": box line width in px, default 8.
--color C / "color": named ffmpeg color (red, blue, green, yellow, orange, magenta, cyan,
  purple, pink, black, white, lime, navy, teal) or #rrggbb, default red. The label background
  uses the same color; use different colors to tell several boxes apart.
flags apply to single-image mode; in a manifest use the per-job fields.

manifest (json array)
  [{"image":"raw/s01.png","box":[x,y,w,h],"label":"Tap Sign in","labelPos":"right","color":"#0066ff","thickness":6,"out":"photographer-step-01-login.png"},
   {"image":"raw/s02.png","out":"photographer-step-02-home.png","boxes":[
      {"box":[40,100,300,80],"label":"Name","labelPos":"inside-top","color":"red"},
      {"box":[40,300,300,80],"label":"Email","labelPos":"below","color":"blue"}]}]
  image: png or jpeg, must exist. out: required. label: optional, max 200 chars.
  box: optional [x,y,w,h] ints in pixels, w,h > 0, fully inside the image.
  boxes: optional list of {box,label,labelPos,color} drawn in one pass; use instead of
  box/label (not both). job-level labelPos/color are defaults for entries that omit them.
  fontsize scales with image width (width/53, min 24). output is deterministic.

output
  one line per written file: "ok <out>". problems go to stderr.
  all input errors are reported before anything is rendered.

exit codes
  0 all ok, 1 validation or render failure (any job), 2 bad usage

example
  capture annotate --manifest shots/manifest.json
  capture annotate --in raw/s01.png --box 880,640,800,120 --label "Tap Sign in" --out out/step-01.png
`
}
