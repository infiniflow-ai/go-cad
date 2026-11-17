package render

import (
	"github.com/unitedrhino/go-cad/internal/drawing"
)

// TextLayout contains text in one coordinate space. Bounds use that space's
// native units; paper-space coordinates are never merged with model space.
type TextLayout struct {
	ID     string
	Bounds [4]float64
	Texts  []SheetText
}

// TextLayouts extracts model-space and paper-space text separately.
// SheetTexts retains its model-space rendering scope.
func TextLayouts(doc *drawing.Document) []TextLayout {
	groups := layoutGroups(doc)
	out := make([]TextLayout, 0, len(groups))
	for _, group := range groups {
		layoutDoc := *doc
		layoutDoc.ModelSpace = group.entities
		layoutDoc.PspaceSpace = nil
		var sheets []Sheet
		if group.space == SpaceModel {
			sheets = DetectSheets(&layoutDoc)
			if len(sheets) == 1 && sheets[0].Name == "整图" {
				sheets = nil
			}
		}
		out = append(out, extractTextLayout(&layoutDoc, group.id, sheets))
	}
	return out
}

func extractTextLayout(doc *drawing.Document, id string, sheets []Sheet) TextLayout {
	prims := drawing.NewTessellator(doc).ExpandAll()
	b := drawing.PrimitivesBounds(prims)
	return TextLayout{ID: id, Bounds: [4]float64{b.MinX, b.MinY, b.MaxX, b.MaxY}, Texts: textsFromPrimitives(doc, prims, sheets)}
}
