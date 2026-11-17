package render

import (
	"bytes"
	"image"
	"image/png"
	"math"
	"testing"

	"github.com/unitedrhino/go-cad/internal/drawing"
	"github.com/unitedrhino/go-cad/internal/entity"
	"github.com/unitedrhino/go-cad/internal/object"
)

func newTestDocument() *drawing.Document {
	return &drawing.Document{
		Blocks:       map[uint64][]any{},
		Attribs:      map[uint64]*entity.EntAttrib{},
		LayerColors:  map[uint64]drawing.LayerColor{},
		InternalObjs: map[uint64]*object.ObjGeneric{},
		ByHandle:     map[uint64]any{},
	}
}

func addPaperLayout(doc *drawing.Document) {
	const paperOwner = uint64(0x200)
	doc.ModelSpace = append(doc.ModelSpace,
		&entity.EntLine{Start: entity.Point3{X: 0, Y: 50, Z: 0}, End: entity.Point3{X: 100, Y: 50, Z: 0}},
		&entity.EntText{Text: "MODEL", Insertion: entity.Point3{X: 10, Y: 60, Z: 0}, Height: 4},
	)
	doc.Blocks[paperOwner] = []any{
		&entity.EntLine{Start: entity.Point3{X: 5, Y: 5, Z: 0}, End: entity.Point3{X: 415, Y: 5, Z: 0}},
		&entity.EntLine{Start: entity.Point3{X: 415, Y: 5, Z: 0}, End: entity.Point3{X: 415, Y: 292, Z: 0}},
		&entity.EntLine{Start: entity.Point3{X: 415, Y: 292, Z: 0}, End: entity.Point3{X: 5, Y: 292, Z: 0}},
		&entity.EntLine{Start: entity.Point3{X: 5, Y: 292, Z: 0}, End: entity.Point3{X: 5, Y: 5, Z: 0}},
		&entity.EntText{Text: "TITLE", Insertion: entity.Point3{X: 330, Y: 20, Z: 0}, Height: 6},
		&entity.EntViewport{
			BaseEntity: entity.BaseEntity{Handle: 0x201},
			Center:     entity.Point3{X: 100, Y: 75, Z: 0},
			Width:      100,
			Height:     80,
			ViewTarget: entity.Point3{X: 50, Y: 50, Z: 0},
			ViewDir:    entity.Point3{X: 0, Y: 0, Z: 1},
			ViewCtr:    entity.Point2{X: 50, Y: 50},
			ViewSize:   100,
			StatusFlag: 1,
		},
	}
	doc.InternalObjs[paperOwner] = &object.ObjGeneric{Name: "BLOCK_HEADER", Fields: []object.ObjField{
		{Key: "name", Val: "*Paper_Space"},
	}}
	doc.InternalObjs[0x300] = &object.ObjGeneric{Name: "LAYOUT", Handles: []uint64{paperOwner}, Fields: []object.ObjField{
		{Key: "layout_name", Val: "Layout A3"},
		{Key: "plotsettings.paper_width", Val: 420.0},
		{Key: "plotsettings.paper_height", Val: 297.0},
	}}
}

func TestLayoutsEnumeratesModelAndPaper(t *testing.T) {
	doc := newTestDocument()
	addPaperLayout(doc)
	layouts := Layouts(doc)
	if len(layouts) != 2 {
		t.Fatalf("Layouts = %d, want 2: %+v", len(layouts), layouts)
	}
	if layouts[0].ID != "model" || layouts[0].Space != SpaceModel {
		t.Fatalf("model layout = %+v", layouts[0])
	}
	paper := layouts[1]
	if paper.ID != "paper:200" || paper.Space != SpacePaper {
		t.Fatalf("paper layout = %+v", paper)
	}
	if paper.Name != "Layout A3" {
		t.Fatalf("paper name = %q, want Layout A3", paper.Name)
	}
	if paper.PaperSize != [2]float64{420, 297} {
		t.Fatalf("paper size = %v", paper.PaperSize)
	}
	if len(paper.Viewports) != 1 {
		t.Fatalf("viewports = %+v", paper.Viewports)
	}
	vp := paper.Viewports[0]
	if vp.Rect != [4]float64{50, 35, 150, 115} {
		t.Fatalf("viewport rect = %v", vp.Rect)
	}
	mapped, ok := paper.ModelBoxToPaper([4]float64{0, 50, 100, 50})
	if !ok {
		t.Fatal("ModelBoxToPaper failed")
	}
	want := [4]float64{60, 75, 140, 75}
	for i := range want {
		if math.Abs(mapped[i]-want[i]) > 1e-9 {
			t.Fatalf("ModelBoxToPaper = %v, want %v", mapped, want)
		}
	}
	if _, ok := LayoutByID(doc, paper.ID); !ok {
		t.Fatal("LayoutByID did not find paper layout")
	}
	if _, ok := LayoutByID(doc, "missing"); ok {
		t.Fatal("LayoutByID found a missing layout")
	}
}

func TestTextLayoutsKeepSpacesSeparate(t *testing.T) {
	doc := newTestDocument()
	addPaperLayout(doc)
	texts := TextLayouts(doc)
	if len(texts) != 2 {
		t.Fatalf("TextLayouts = %d, want 2", len(texts))
	}
	byID := map[string]TextLayout{}
	for _, l := range texts {
		byID[l.ID] = l
	}
	model := byID["model"]
	if len(model.Texts) != 1 || model.Texts[0].Text != "MODEL" {
		t.Fatalf("model texts = %+v", model.Texts)
	}
	paper := byID["paper:200"]
	if len(paper.Texts) != 1 || paper.Texts[0].Text != "TITLE" {
		t.Fatalf("paper texts = %+v", paper.Texts)
	}
	if paper.Bounds[0] < 0 || paper.Bounds[2] > 500 || paper.Bounds[2] <= 300 {
		t.Fatalf("paper bounds mixed with model coordinates: %v", paper.Bounds)
	}
}

func TestRenderLayoutPNGComposesViewport(t *testing.T) {
	doc := newTestDocument()
	addPaperLayout(doc)
	res, err := RenderLayoutPNG(doc, "paper:200", RenderOptions{Width: 840})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(res.Data, []byte("\x89PNG")) {
		t.Fatal("render did not return a PNG")
	}
	if res.Width != 840 || res.Height <= 0 {
		t.Fatalf("size = %dx%d", res.Width, res.Height)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v", res.Warnings)
	}
	img := decodePNG(t, res.Data)
	if !pixelAt(img, res.Viewport, res.Width, res.Height, 100, 75) {
		t.Fatal("model VIEWPORT line was not composited at its paper position")
	}
	if pixelAt(img, res.Viewport, res.Width, res.Height, 50, 75) {
		t.Fatal("unexpected ink outside the VIEWPORT rectangle")
	}
	if !pixelNear(img, res.Viewport, res.Width, res.Height, 335, 23, 6) {
		t.Fatal("paper-space title text was not rendered")
	}
}

func TestRenderLayoutBoxPNGReturnsActualViewport(t *testing.T) {
	doc := newTestDocument()
	addPaperLayout(doc)
	box := [4]float64{95, 70, 145, 80}
	res, err := RenderLayoutBoxPNG(doc, "paper:200", box, RenderOptions{Width: 600})
	if err != nil {
		t.Fatal(err)
	}
	want := SheetViewport(box)
	if res.Viewport != want {
		t.Fatalf("viewport = %v, want %v", res.Viewport, want)
	}
	if res.Width != 600 || res.Height <= 0 {
		t.Fatalf("size = %dx%d", res.Width, res.Height)
	}
	img := decodePNG(t, res.Data)
	if !pixelAt(img, res.Viewport, res.Width, res.Height, 100, 75) {
		t.Fatal("cropped layout lost the model line inside the box")
	}
}

func TestComposeViewportSkips3D(t *testing.T) {
	prims := []drawing.Primitive{{Kind: 0, Strokes: []drawing.Stroke{{X1: 0, Y1: 0, X2: 1, Y2: 0}}}}
	vp := LayoutViewport{
		Center: [2]float64{10, 10}, Width: 20, Height: 20,
		ViewCenter: [2]float64{0, 0}, ViewSize: 10,
		ViewDir: [3]float64{1, -1, 1}, Status: 1, Active: true,
		Rect: [4]float64{0, 0, 20, 20},
	}
	out, warnings := composeViewport(prims, vp, [2]float64{0, 0})
	if len(out) != 0 {
		t.Fatalf("3D viewport produced %d primitives", len(out))
	}
	if len(warnings) == 0 {
		t.Fatal("3D viewport was skipped without a warning")
	}
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func pixelNear(img image.Image, viewport [4]float64, width, height int, x, y float64, radius int) bool {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if pixelAt(img, viewport, width, height, x+float64(dx), y+float64(dy)) {
				return true
			}
		}
	}
	return false
}

func pixelAt(img image.Image, viewport [4]float64, width, height int, x, y float64) bool {
	px := int(math.Floor((x - viewport[0]) / (viewport[2] - viewport[0]) * float64(width)))
	py := int(math.Floor((viewport[3] - y) / (viewport[3] - viewport[1]) * float64(height)))
	if px < 0 || px >= width || py < 0 || py >= height {
		return false
	}
	r, g, b, _ := img.At(px, py).RGBA()
	return r < 0x8000 || g < 0x8000 || b < 0x8000
}
