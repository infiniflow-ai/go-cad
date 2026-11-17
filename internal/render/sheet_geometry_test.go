package render

import (
	"testing"

	"github.com/unitedrhino/go-cad/internal/entity"
)

func TestGeometryFramesDetected(t *testing.T) {
	doc := newTestDocument()
	addFrame := func(x, y float64, title string) {
		doc.ModelSpace = append(doc.ModelSpace, &entity.EntLwPolyline{
			Vertices: []entity.Point2{
				{X: x, Y: y},
				{X: x + 297, Y: y},
				{X: x + 297, Y: y + 210},
				{X: x, Y: y + 210},
			},
		})
		doc.ModelSpace = append(doc.ModelSpace, &entity.EntText{
			Text:      title,
			Insertion: entity.Point3{X: x + 250, Y: y + 20, Z: 0},
			Height:    8,
		})
		for i := 0; i < 10; i++ {
			doc.ModelSpace = append(doc.ModelSpace, &entity.EntLine{
				Start: entity.Point3{X: x + 20 + float64(i)*10, Y: y + 60, Z: 0},
				End:   entity.Point3{X: x + 25 + float64(i)*10, Y: y + 90, Z: 0},
			})
		}
	}
	addFrame(0, 0, "SHEET A")
	addFrame(400, 0, "SHEET B")

	det := DetectSheetsDetailed(doc)
	if det.Fallback {
		t.Fatalf("geometry frames fell back: %+v", det)
	}
	if len(det.Sheets) != 2 {
		t.Fatalf("sheets = %d, want 2: %+v", len(det.Sheets), det.Sheets)
	}
	for _, sheet := range det.Sheets {
		if sheet.Source != "polyline" {
			t.Errorf("sheet %q source = %q", sheet.Name, sheet.Source)
		}
		if sheet.LayoutID != "model" || sheet.Space != SpaceModel {
			t.Errorf("sheet %q layout = %q/%q", sheet.Name, sheet.LayoutID, sheet.Space)
		}
		if sheet.Confidence < 0.8 {
			t.Errorf("sheet %q confidence = %.2f", sheet.Name, sheet.Confidence)
		}
		if sheet.Name != "SHEET A" && sheet.Name != "SHEET B" {
			t.Errorf("sheet name = %q", sheet.Name)
		}
	}
}

func TestGeometryFrameRejectsRoomOutline(t *testing.T) {
	doc := newTestDocument()
	doc.ModelSpace = append(doc.ModelSpace, &entity.EntLwPolyline{
		Vertices: []entity.Point2{
			{X: 0, Y: 0},
			{X: 400, Y: 0},
			{X: 400, Y: 230},
			{X: 0, Y: 230},
		},
	})
	doc.ModelSpace = append(doc.ModelSpace, &entity.EntText{
		Text:      "ROOM",
		Insertion: entity.Point3{X: 340, Y: 20, Z: 0},
		Height:    8,
	})
	for i := 0; i < 10; i++ {
		doc.ModelSpace = append(doc.ModelSpace, &entity.EntLine{
			Start: entity.Point3{X: 10 + float64(i)*10, Y: 80, Z: 0},
			End:   entity.Point3{X: 15 + float64(i)*10, Y: 100, Z: 0},
		})
	}
	det := DetectSheetsDetailed(doc)
	if !det.Fallback || len(det.Sheets) != 1 || det.Sheets[0].Name != "整图" {
		t.Fatalf("room outline accepted as sheet: %+v", det)
	}
	if det.Sheets[0].Source != "fallback" || det.Sheets[0].Confidence != 0 {
		t.Fatalf("fallback metadata = %+v", det.Sheets[0])
	}
}
