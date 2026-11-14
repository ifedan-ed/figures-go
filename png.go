package figures

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ifedan-ed/figures-go/internal/core"
	"github.com/ifedan-ed/figures-go/internal/page"
)

// Rasteriser makes every figure a PNG, for the apps (and, for HTML and
// Mermaid figures, for pages too). react-native-svg draws only part of
// SVG, differently on iOS and Android, and an app cannot run Mermaid or
// lay out HTML without a web view; a PNG made here is the same picture on
// every phone, and is exactly what the tests and the gallery look at.
//
// Typed and free-form SVG figures are drawn by resvg
// (github.com/linebender/resvg, Apache-2.0 OR MIT): one static binary,
// pinned in each service's Dockerfile, with fonts from FontDir only
// (fonts/README): FreeSans, whose widths are Helvetica's, so text fits
// where the layout measured it, and DejaVu Sans for anything it lacks.
// HTML and Mermaid figures are drawn by Chromium, through a pinned
// Gotenberg 8 (its screenshot route), from a page that can load nothing.
type Rasteriser struct {
	// Bin is the resvg binary.
	Bin string
	// FontDir holds the fonts resvg draws with.
	FontDir string
	// Chromium is the Gotenberg 8 base URL ("http://figures-chromium:3000").
	Chromium string
	// Cache keeps PNGs by what they were drawn from, so a figure is drawn
	// once however often it is shown.
	Cache Cache
	// HTTP calls Chromium; nil is http.DefaultClient.
	HTTP *http.Client
	// Timeout is per PNG; 0 is 20 seconds.
	Timeout time.Duration
}

// Cache keeps PNGs. Keys are given by the rasteriser (CacheKeyPrefix and a
// hash of everything the PNG depends on), so an entry never goes stale; it
// only expires.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, png []byte) error
}

// CacheKeyPrefix begins every cache key.
const CacheKeyPrefix = "figures:png:v1:"

// PNGWidth is the PNG's width in pixels: two and a half times the
// figure's units, sharp at phone width and when opened full screen.
const PNGWidth = 1000

// maxPNG is the largest PNG kept.
const maxPNG = 3 << 20

// Check says whether the rasteriser can run: resvg runs, the fonts are
// there, Chromium answers as a Gotenberg 8, and there is a cache. A
// service checks it once, at start, and does not start without it.
func (r *Rasteriser) Check(ctx context.Context) error {
	if r == nil || r.Bin == "" || r.FontDir == "" || r.Chromium == "" || r.Cache == nil {
		return errors.New("figures: the rasteriser needs resvg, its font directory, Chromium's address and a cache")
	}
	if err := exec.CommandContext(ctx, r.Bin, "--version").Run(); err != nil {
		return fmt.Errorf("figures: %s does not run: %w", r.Bin, err)
	}
	fonts, _ := filepath.Glob(filepath.Join(r.FontDir, "*.ttf"))
	if len(fonts) == 0 {
		return fmt.Errorf("figures: no .ttf fonts in %s", r.FontDir)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(r.Chromium, "/")+"/version", nil)
	if err != nil {
		return err
	}
	res, err := r.client().Do(req)
	if err != nil {
		return fmt.Errorf("figures: Chromium at %s: %w", r.Chromium, err)
	}
	defer res.Body.Close()
	v, _ := io.ReadAll(io.LimitReader(res.Body, 100))
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(string(v), "8.") {
		return fmt.Errorf("figures: %s is not a Gotenberg 8 (status %d, version %q)", r.Chromium, res.StatusCode, v)
	}
	return nil
}

// Shown is a figure as it is sent to a page or an app: the figure, and its
// PNGs (base64). PNGs are made (or found in the cache) when a figure is
// sent, never kept with it: what is kept is the Figure.
type Shown struct {
	Figure
	PNG     string `json:"png"`
	PNGDark string `json:"png_dark"`
}

// Draw is the tool's call: draw_figure's arguments checked and drawn, with
// its PNGs. A *Refusal is the model's to correct, whether the arguments
// were refused here or the drawing failed in the browser (a Mermaid
// syntax error, a diagram too wide). Keep the Shown's Figure.
func (r *Rasteriser) Draw(ctx context.Context, args []byte) (*Shown, error) {
	f, err := Render(args)
	if err != nil {
		return nil, err
	}
	return r.Show(ctx, f)
}

// Show is f with its PNGs, drawn from what f keeps: its SVGs, or for a
// browser-drawn figure its spec.
func (r *Rasteriser) Show(ctx context.Context, f *Figure) (*Shown, error) {
	out := &Shown{Figure: *f}
	switch f.Form {
	case "svg":
		light, err := r.svgPNG(ctx, f.SVG)
		if err != nil {
			return nil, err
		}
		dark, err := r.svgPNG(ctx, f.SVGDark)
		if err != nil {
			return nil, err
		}
		out.PNG, out.PNGDark = b64(light), b64(dark)
	case "png":
		pg, title, caption, err := pageOf(f)
		if err != nil {
			return nil, err
		}
		var h int
		for _, t := range []*core.Theme{&core.Light, &core.Dark} {
			img, height, err := r.pagePNG(ctx, pg.HTML(t, title, caption), pg.Scripted())
			if err != nil {
				var ref *Refusal
				if errors.As(err, &ref) {
					ref.Type = f.Type
				}
				return nil, err
			}
			h = height
			if t == &core.Light {
				out.PNG = b64(img)
			} else {
				out.PNGDark = b64(img)
			}
		}
		out.Width = core.Width
		out.Height = float64(h) / page.Zoom
	default:
		return nil, fmt.Errorf("figures: form %q", f.Form)
	}
	return out, nil
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// pageOf is a browser-drawn figure's page, read again from its spec.
func pageOf(f *Figure) (core.Page, string, string, error) {
	var a args
	if err := json.Unmarshal(f.Spec, &a); err != nil {
		return nil, "", "", fmt.Errorf("figures: the kept spec: %w", err)
	}
	k, ok := kinds[a.Type]
	if !ok || k.PageOf == nil {
		return nil, "", "", fmt.Errorf("figures: %q is not drawn by a browser", a.Type)
	}
	pg, err := k.PageOf(a.Spec)
	if err != nil {
		return nil, "", "", err
	}
	return pg, strings.TrimSpace(a.Title), strings.TrimSpace(a.Caption), nil
}

func (r *Rasteriser) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return http.DefaultClient
}

func (r *Rasteriser) timeout() time.Duration {
	if r.Timeout == 0 {
		return 20 * time.Second
	}
	return r.Timeout
}

func key(kind, source string) string {
	h := sha256.Sum256([]byte(kind + "\x00" + page.MermaidVersion + "\x00" + source))
	return CacheKeyPrefix + kind + ":" + hex.EncodeToString(h[:])
}

// cached returns the cached PNG for k, or makes it with draw and keeps it.
// The PNG's height in pixels is stored in front of it for browser-drawn
// figures, whose height is known only once drawn.
func (r *Rasteriser) cached(ctx context.Context, k string, draw func() ([]byte, error)) ([]byte, error) {
	if b, ok, err := r.Cache.Get(ctx, k); err != nil {
		return nil, fmt.Errorf("figures: cache: %w", err)
	} else if ok && bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		return b, nil
	}
	b, err := draw()
	if err != nil {
		return nil, err
	}
	if err := r.Cache.Set(ctx, k, b); err != nil {
		return nil, fmt.Errorf("figures: cache: %w", err)
	}
	return b, nil
}

// svgPNG draws an SVG Render made as a PNG PNGWidth pixels wide.
func (r *Rasteriser) svgPNG(ctx context.Context, svg string) ([]byte, error) {
	return r.svgPNGAt(ctx, svg, PNGWidth)
}

// svgPNGAt draws an SVG figure as a PNG width pixels wide.
func (r *Rasteriser) svgPNGAt(ctx context.Context, svg string, width int) ([]byte, error) {
	if len(svg) > MaxSVGBytes {
		return nil, fmt.Errorf("figures: svg is %d bytes", len(svg))
	}
	kind := "svg"
	if width != PNGWidth {
		kind = "svg@" + strconv.Itoa(width)
	}
	return r.cached(ctx, key(kind, svg), func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, r.timeout())
		defer cancel()
		cmd := exec.CommandContext(ctx, r.Bin,
			"--width", strconv.Itoa(width),
			"--skip-system-fonts", "--use-fonts-dir", r.FontDir,
			"--font-family", "FreeSans", "--sans-serif-family", "FreeSans",
			"--shape-rendering", "geometricPrecision", "--text-rendering", "optimizeLegibility",
			"-", "-c")
		cmd.Stdin = strings.NewReader(svg)
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		cmd.Env = []string{"HOME=" + os.TempDir()}
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("figures: resvg: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
		}
		return checkPNG(out.Bytes())
	})
}

func checkPNG(b []byte) ([]byte, error) {
	if !bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, errors.New("figures: not a PNG")
	}
	if len(b) > maxPNG {
		return nil, fmt.Errorf("figures: the PNG is %d bytes, more than %d", len(b), maxPNG)
	}
	return b, nil
}

// screenHeight is the browser's window: as tall as a figure may be.
var screenHeight = int(core.MaxHeight * page.Zoom)

// pagePNG has Chromium draw a page, cut to the figure's own height. It
// returns the PNG and its height in pixels.
func (r *Rasteriser) pagePNG(ctx context.Context, html string, scripted bool) ([]byte, int, error) {
	img, err := r.cached(ctx, key("page", html), func() ([]byte, error) {
		shot, err := r.screenshot(ctx, html, scripted)
		if err != nil {
			return nil, err
		}
		return crop(shot)
	})
	if err != nil {
		return nil, 0, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil, 0, fmt.Errorf("figures: %w", err)
	}
	return img, cfg.Height, nil
}

// screenshot is Gotenberg's Chromium screenshot of the page. A page that
// throws (Mermaid could not read the diagram; it is too wide) is the
// model's to correct: its message comes back as a *Refusal.
func (r *Rasteriser) screenshot(ctx context.Context, html string, scripted bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write([]byte(html)); err != nil {
		return nil, err
	}
	fields := [][2]string{{"width", strconv.Itoa(PNGWidth)}, {"height", strconv.Itoa(screenHeight)}, {"format", "png"},
		{"failOnConsoleExceptions", "true"}, {"skipNetworkIdleEvent", "true"}}
	if scripted {
		fields = append(fields, [2]string{"waitForExpression", "window.figureReady === true"})
	}
	for _, f := range fields {
		if err := w.WriteField(f[0], f[1]); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.Chromium, "/")+"/forms/chromium/screenshot/html", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := r.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("figures: Chromium: %w", err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("figures: Chromium: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		text := string(b)
		if i := strings.Index(text, "FIGURE: "); i >= 0 {
			msg := text[i+len("FIGURE: "):]
			if j := strings.IndexAny(msg, "\n"); j >= 0 {
				msg = msg[:j]
			}
			msg = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(msg), ":"))
			return nil, &Refusal{Problems: []string{msg}}
		}
		return nil, fmt.Errorf("figures: Chromium: status %d: %.300s", res.StatusCode, text)
	}
	return checkPNG(b)
}

// crop cuts a screenshot to the figure: down to its last row that is not
// the background, with the frame's margin below. A figure that reaches the
// bottom of the window is too tall, and refused.
func crop(shot []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		return nil, fmt.Errorf("figures: %w", err)
	}
	b := img.Bounds()
	bg := img.At(b.Min.X, b.Max.Y-1)
	last := -1
	for y := b.Max.Y - 1; y >= b.Min.Y && last < 0; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !same(img.At(x, y), bg) {
				last = y
				break
			}
		}
	}
	if last < 0 {
		return nil, &Refusal{Problems: []string{"the figure came out empty"}}
	}
	if last >= b.Max.Y-int(14*page.Zoom) {
		return nil, &Refusal{Problems: []string{fmt.Sprintf("the figure is taller than %.0f units, more than can be followed on a phone; show less or split it", core.MaxHeight)}}
	}
	h := min(b.Max.Y, last+1+int(14*page.Zoom))
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, errors.New("figures: the screenshot cannot be cut")
	}
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, sub.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+h))); err != nil {
		return nil, err
	}
	return checkPNG(out.Bytes())
}

func same(a, b color.Color) bool {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	d := func(x, y uint32) uint32 {
		if x > y {
			return x - y
		}
		return y - x
	}
	return d(r1, r2) < 0x300 && d(g1, g2) < 0x300 && d(b1, b2) < 0x300
}

// MemoryCache is a Cache in memory, for tests and for the preview command.
type MemoryCache map[string][]byte

func (m MemoryCache) Get(_ context.Context, k string) ([]byte, bool, error) {
	b, ok := m[k]
	return b, ok, nil
}

func (m MemoryCache) Set(_ context.Context, k string, b []byte) error {
	m[k] = b
	return nil
}
