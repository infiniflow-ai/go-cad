package render

import (
	"testing"

	"github.com/unitedrhino/go-cad/internal/entity"
)

type integritySheetSpec = struct {
	h                       uint64
	name                    string
	title                   string
	offset                  [2]float64
	W, hgt, scale, rotation float64
}

func TestDetectSheetsRejectsSmallCoveredFragment(t *testing.T) {
	doc := buildSheetDoc(t, []integritySheetSpec{aSeriesSheet(100, "ISO_A3_FRAME", "TITLE", [2]float64{})})
	for i := 0; i < 40; i++ {
		doc.ModelSpace = append(doc.ModelSpace, &entity.EntText{Text: "OUTSIDE CONTENT", Insertion: entity.Point3{X: 2000 + float64(i%8)*500, Y: 1500 + float64(i/8)*300}, Height: 20})
	}
	result := DetectSheetsDetailed(doc)
	if !result.Fallback || len(result.Sheets) != 1 || result.Sheets[0].Source != "fallback" {
		t.Fatalf("small legend frame hid the principal model content: %+v", result)
	}
}

func TestSheetRenderViewportCoversOwnedText(t *testing.T) {
	doc := buildSheetDoc(t, []integritySheetSpec{
		aSeriesSheet(100, "ISO_A3_FRAME", "LEFT", [2]float64{}),
		aSeriesSheet(200, "ISO_A3_FRAME", "RIGHT", [2]float64{1300, 0}),
	})
	const label = "ANNOTATION EXTENDS ACROSS FRAME"
	doc.ModelSpace = append(doc.ModelSpace, &entity.EntText{Text: label, Insertion: entity.Point3{X: 1150, Y: 400}, Height: 40})
	detection := DetectSheetsDetailed(doc)
	layouts := TextLayouts(doc)
	results, err := RenderAllSheets(doc, RenderOptions{Width: 400}, "svg")
	if err != nil {
		t.Fatal(err)
	}
	for _, layout := range layouts {
		for _, text := range layout.Texts {
			if text.Text != label {
				continue
			}
			if text.Sheet < 1 || text.Sheet > len(detection.Sheets) {
				t.Fatalf("text lost sheet membership: %+v", text)
			}
			name := detection.Sheets[text.Sheet-1].Name
			for _, result := range results {
				if result.Name != name {
					continue
				}
				b, v := text.Bounds, result.Viewport
				if b[0] < v[0] || b[1] < v[1] || b[2] > v[2] || b[3] > v[3] {
					t.Fatalf("owned glyph clipped: bbox=%v viewport=%v", b, v)
				}
				return
			}
			t.Fatalf("missing rendered sheet %q", name)
		}
	}
	t.Fatal("fixture annotation missing")
}
