package render

import (
	"fmt"
	"github.com/unitedrhino/go-cad/internal/drawing"
	"github.com/unitedrhino/go-cad/internal/entity"
	"sort"
	"strings"
)

// TextLayout contains text in one coordinate space. Bounds use that space's
// native units; paper-space coordinates are never merged with model space.
type TextLayout struct {
	ID     string
	Bounds [4]float64
	Texts  []SheetText
}

// TextLayouts extracts model-space sheets and paper-space layouts separately.
// SheetTexts retains its model-space rendering scope.
func TextLayouts(doc *drawing.Document) []TextLayout {
	if doc == nil {
		return nil
	}
	model := *doc
	model.PspaceSpace = nil
	sheets := DetectSheets(&model)
	if len(sheets) == 1 && sheets[0].Name == "整图" {
		sheets = nil
	}
	out := []TextLayout{extractTextLayout(&model, "model", sheets)}
	groups := map[uint64][]any{}
	seen := map[uint64]bool{}
	add := func(owner uint64, ent any) {
		base := entity.EntityBase(ent)
		if base == nil {
			return
		}
		if base.Handle != 0 {
			if seen[base.Handle] {
				return
			}
			seen[base.Handle] = true
		}
		groups[owner] = append(groups[owner], ent)
	}
	handles := make([]uint64, 0, len(doc.ByHandle))
	for handle := range doc.ByHandle {
		handles = append(handles, handle)
	}
	sort.Slice(handles, func(i, j int) bool { return handles[i] < handles[j] })
	for _, handle := range handles {
		ent := doc.ByHandle[handle]
		base := entity.EntityBase(ent)
		if base != nil && base.Mode == 1 {
			add(base.Owner, ent)
		}
	}
	for _, ent := range doc.PspaceSpace {
		base := entity.EntityBase(ent)
		if base != nil {
			add(base.Owner, ent)
		}
	}
	for owner, ents := range doc.Blocks {
		obj := doc.InternalObjs[owner]
		if obj == nil {
			continue
		}
		name, _ := obj.Field("name").(string)
		if !strings.HasPrefix(strings.ToLower(name), "*paper_space") && !strings.EqualFold(name, "$Paper_Space") {
			continue
		}
		for _, ent := range ents {
			add(owner, ent)
		}
	}
	owners := make([]uint64, 0, len(groups))
	for owner := range groups {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i] < owners[j] })
	for _, owner := range owners {
		paper := model
		paper.ModelSpace = groups[owner]
		// The same block definitions remain available for INSERT transforms.
		out = append(out, extractTextLayout(&paper, fmt.Sprintf("paper:%x", owner), nil))
	}
	return out
}

func extractTextLayout(doc *drawing.Document, id string, sheets []Sheet) TextLayout {
	prims := drawing.NewTessellator(doc).ExpandAll()
	b := sheetContentBounds(prims)
	return TextLayout{ID: id, Bounds: [4]float64{b.MinX, b.MinY, b.MaxX, b.MaxY}, Texts: textsFromPrimitives(doc, prims, sheets)}
}
