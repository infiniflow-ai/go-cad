package cad

import (
	"github.com/unitedrhino/go-cad/internal/drawing"
	"github.com/unitedrhino/go-cad/internal/entity"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextLayoutsSeparateCoordinateSpaces(t *testing.T) {
	model := &entity.EntText{BaseEntity: entity.BaseEntity{Handle: 1, Mode: 2}, Text: "MODEL", Insertion: entity.Point3{X: 10, Y: 10}, Height: 5}
	paper := &entity.EntText{BaseEntity: entity.BaseEntity{Handle: 2, Mode: 1}, Text: "PAPER", Insertion: entity.Point3{X: 10, Y: 10}, Height: 2}
	doc := &drawing.Document{ModelSpace: []any{model}, ByHandle: map[uint64]any{1: model, 2: paper}, Blocks: map[uint64][]any{}}
	layouts := TextLayouts(doc)
	seen := map[string]string{}
	for _, layout := range layouts {
		for _, text := range layout.Texts {
			seen[text.Text] = layout.ID
		}
	}
	if seen["MODEL"] == "" || seen["PAPER"] == "" || seen["MODEL"] == seen["PAPER"] {
		t.Fatalf("coordinate spaces mixed or lost: %v", seen)
	}
	if len(SheetTexts(doc)) != 1 {
		t.Fatal("model-space rendering scope changed")
	}
}

func TestTextLayoutsRealPaperSpace(t *testing.T) {
	root := os.Getenv("CAD_CORPUS_DIR")
	if root == "" {
		t.Skip("CAD_CORPUS_DIR not set")
	}
	for _, tc := range []struct{ name, want string }{{"pulley_drive_3d.dwg", "ISO 2768"}, {"two_storey_house_3d.dwg", "Interpiano"}, {"apartment_floor_plan.dwg", "SUPERFICI UTILI NETTE"}} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, "dwg", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, layout := range TextLayouts(doc) {
				if layout.ID == "model" {
					continue
				}
				for _, text := range layout.Texts {
					if strings.Contains(text.Text, tc.want) {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("paper-space text %q not extracted", tc.want)
			}
		})
	}
}

func TestTextLayoutsRealBinaryDXFPaperSpace(t *testing.T) {
	root := os.Getenv("CAD_CORPUS_DIR")
	if root == "" {
		t.Skip("CAD_CORPUS_DIR not set")
	}
	for _, tc := range []struct{ name, want string }{{"pulley_drive_3d.dxfb", "ISO 2768"}, {"two_storey_house_3d.dxfb", "Interpiano"}, {"apartment_floor_plan.dxfb", "SUPERFICI UTILI NETTE"}} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, "dxfb", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := ParseDXF(data)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, layout := range TextLayouts(doc) {
				if layout.ID == "model" {
					continue
				}
				for _, text := range layout.Texts {
					if strings.Contains(text.Text, tc.want) {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("binary DXF paper-space text %q not extracted", tc.want)
			}
		})
	}
}

func TestTextLayoutsRetainRotatedTextBounds(t *testing.T) {
	text := &entity.EntText{BaseEntity: entity.BaseEntity{Handle: 1, Mode: 2}, Text: "ROTATED", Insertion: entity.Point3{X: 10, Y: 20}, Height: 5, Rotation: 1.5707963267948966}
	doc := &drawing.Document{ModelSpace: []any{text}, Blocks: map[uint64][]any{}}
	layouts := TextLayouts(doc)
	if len(layouts) == 0 || len(layouts[0].Texts) != 1 {
		t.Fatal("missing rotated text")
	}
	b := layouts[0].Texts[0].Bounds
	if b[3] <= 20 || b[0] > 10 || b[1] > 20 {
		t.Fatalf("rotated text bounds do not cover native position: %v", b)
	}
}
