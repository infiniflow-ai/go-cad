package render

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/unitedrhino/go-cad/internal/drawing"
	"github.com/unitedrhino/go-cad/internal/entity"
)

// SpaceKind identifies the coordinate space a Layout belongs to. Model and
// paper layouts use separate native coordinate systems; renderers and callers
// must never mix their bounds.
type SpaceKind string

const (
	SpaceModel SpaceKind = "model"
	SpacePaper SpaceKind = "paper"
)

// LayoutViewport describes one VIEWPORT entity inside a paper layout.
// Rect is the paper-space rectangle the viewport occupies. Center/Width/Height
// are the raw paper-space VIEWPORT fields; ViewCenter/ViewSize describe the
// model-space view.
type LayoutViewport struct {
	Handle     uint64     `json:"handle"`
	Center     [2]float64 `json:"center"`
	Width      float64    `json:"width"`
	Height     float64    `json:"height"`
	ViewCenter [2]float64 `json:"view_center"`
	ViewSize   float64    `json:"view_size"`
	ViewTwist  float64    `json:"view_twist"`
	ViewDir    [3]float64 `json:"view_dir"`
	Status     uint32     `json:"status"`
	Active     bool       `json:"active"`
	Rect       [4]float64 `json:"rect"`
}

// Layout is one drawing layout: either the model layout or a paper-space
// layout. Bounds use the layout's native coordinates. PaperSize is populated
// for paper layouts when the LAYOUT object exposes a paper size.
type Layout struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Space     SpaceKind        `json:"space"`
	Bounds    [4]float64       `json:"bounds"`
	PaperSize [2]float64       `json:"paper_size"`
	Viewports []LayoutViewport `json:"viewports"`
	Entities  int              `json:"entities"`
}

// layoutGroup is the internal entity carrier behind a Layout.
type layoutGroup struct {
	id        string
	name      string
	space     SpaceKind
	owner     uint64
	entities  []any
	bounds    [4]float64
	hasBounds bool
	paperSize [2]float64
	viewports []LayoutViewport
}

// Layouts enumerates the model layout and every paper-space layout in a
// stable order (model first, then paper layouts by block handle).
func Layouts(doc *drawing.Document) []Layout {
	groups := layoutGroups(doc)
	out := make([]Layout, 0, len(groups))
	for _, g := range groups {
		bounds := g.bounds
		if !g.hasBounds {
			bounds = [4]float64{0, 0, 0, 0}
		}
		out = append(out, Layout{
			ID:        g.id,
			Name:      g.name,
			Space:     g.space,
			Bounds:    bounds,
			PaperSize: g.paperSize,
			Viewports: g.viewports,
			Entities:  len(g.entities),
		})
	}
	return out
}

// LayoutByID returns the layout with the given stable ID.
func LayoutByID(doc *drawing.Document, id string) (Layout, bool) {
	for _, l := range Layouts(doc) {
		if l.ID == id {
			return l, true
		}
	}
	return Layout{}, false
}

// layoutGroups collects entities per layout without mixing coordinate spaces.
// Entity collection matches the original TextLayouts behavior: model entities
// come from ModelSpace only; paper entities come from mode==1 records, the
// pre-R13 PspaceSpace list, and *Paper_Space block definitions, deduplicated
// by handle.
func layoutGroups(doc *drawing.Document) []layoutGroup {
	if doc == nil {
		return nil
	}
	model := *doc
	model.PspaceSpace = nil
	modelGroup := layoutGroup{id: "model", name: "模型", space: SpaceModel, entities: model.ModelSpace}
	modelGroup.bounds, modelGroup.hasBounds = layoutContentBounds(&model, SpaceModel)
	groups := []layoutGroup{modelGroup}

	type paperCollect struct {
		owner    uint64
		entities []any
	}
	byOwner := map[uint64]*paperCollect{}
	var ownerOrder []uint64
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
		c := byOwner[owner]
		if c == nil {
			c = &paperCollect{owner: owner}
			byOwner[owner] = c
			ownerOrder = append(ownerOrder, owner)
		}
		c.entities = append(c.entities, ent)
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
		if !isPaperSpaceBlockName(name) {
			continue
		}
		for _, ent := range ents {
			add(owner, ent)
		}
	}
	sort.Slice(ownerOrder, func(i, j int) bool { return ownerOrder[i] < ownerOrder[j] })

	layouts := paperLayoutInfo(doc)
	for _, owner := range ownerOrder {
		c := byOwner[owner]
		info := layouts[owner]
		group := layoutGroup{
			id:        fmt.Sprintf("paper:%x", owner),
			name:      info.name,
			space:     SpacePaper,
			owner:     owner,
			entities:  c.entities,
			paperSize: info.paperSize,
		}
		if group.name == "" {
			group.name = paperLayoutFallbackName(doc, owner)
		}
		copyDoc := model
		copyDoc.ModelSpace = c.entities
		copyDoc.PspaceSpace = nil
		group.bounds, group.hasBounds = layoutContentBounds(&copyDoc, SpacePaper)
		group.viewports = collectLayoutViewports(c.entities)
		groups = append(groups, group)
	}
	return groups
}

// layoutMeta is the LAYOUT-object metadata keyed by block-header handle.
type layoutMeta struct {
	name      string
	paperSize [2]float64
}

func paperLayoutInfo(doc *drawing.Document) map[uint64]layoutMeta {
	out := map[uint64]layoutMeta{}
	for _, obj := range doc.InternalObjs {
		if obj == nil || obj.Name != "LAYOUT" || len(obj.Handles) == 0 {
			continue
		}
		name, _ := obj.Field("layout_name").(string)
		w, _ := obj.Field("plotsettings.paper_width").(float64)
		h, _ := obj.Field("plotsettings.paper_height").(float64)
		out[obj.Handles[0]] = layoutMeta{name: strings.TrimSpace(name), paperSize: [2]float64{w, h}}
	}
	return out
}

// paperLayoutFallbackName derives a readable name from the paper-space block
// name when no LAYOUT object is available (DXF input or older files).
func paperLayoutFallbackName(doc *drawing.Document, owner uint64) string {
	obj := doc.InternalObjs[owner]
	if obj != nil {
		if name, _ := obj.Field("name").(string); name != "" {
			lower := strings.ToLower(name)
			switch {
			case lower == "*paper_space" || lower == "$paper_space":
				return "布局 1"
			case strings.HasPrefix(lower, "*paper_space"):
				if n, err := strconv.Atoi(name[len("*Paper_Space"):]); err == nil {
					return fmt.Sprintf("布局 %d", n+2)
				}
			}
			return name
		}
	}
	return fmt.Sprintf("布局 %x", owner)
}

// isPaperSpaceBlockName reports whether a block name denotes paper space.
func isPaperSpaceBlockName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(lower, "*paper_space") || strings.EqualFold(lower, "$paper_space")
}

// collectLayoutViewports extracts VIEWPORT entities from a paper layout.
func collectLayoutViewports(ents []any) []LayoutViewport {
	var out []LayoutViewport
	for _, ent := range ents {
		vp, ok := ent.(*entity.EntViewport)
		if !ok {
			continue
		}
		w, h := vp.Width, vp.Height
		if w <= 0 || h <= 0 {
			continue
		}
		viewSize := vp.ViewSize
		if viewSize <= 0 {
			viewSize = h
		}
		out = append(out, LayoutViewport{
			Handle:     vp.Handle,
			Center:     [2]float64{vp.Center.X, vp.Center.Y},
			Width:      w,
			Height:     h,
			ViewCenter: [2]float64{vp.ViewCtr.X, vp.ViewCtr.Y},
			ViewSize:   viewSize,
			ViewTwist:  vp.ViewTwist,
			ViewDir:    [3]float64{vp.ViewDir.X, vp.ViewDir.Y, vp.ViewDir.Z},
			Status:     vp.StatusFlag,
			Active:     vp.StatusFlag != 0,
			Rect:       [4]float64{vp.Center.X - w/2, vp.Center.Y - h/2, vp.Center.X + w/2, vp.Center.Y + h/2},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out
}

// PaperRepresentation reports whether this VIEWPORT is the synthetic
// paper-space view (its view center/size equal the paper viewport itself).
// Such viewports describe the sheet, not model content, and must not be
// composited as model views.
func (v LayoutViewport) PaperRepresentation() bool {
	if v.ViewSize <= 0 || v.Height <= 0 {
		return false
	}
	return math.Abs(v.ViewSize-v.Height) <= 0.02*math.Max(1, v.Height) &&
		math.Hypot(v.ViewCenter[0]-v.Center[0], v.ViewCenter[1]-v.Center[1]) <= 0.02*math.Max(v.Width, v.Height)
}

// Supported reports whether this viewport can be composited by the 2D
// renderer (active, rectangular, top view, valid view size).
func (v LayoutViewport) Supported() bool {
	if !v.Active || v.Width <= 0 || v.Height <= 0 || v.ViewSize <= 0 {
		return false
	}
	return math.Abs(v.ViewDir[2]) >= 0.99 && math.Abs(v.ViewDir[0]) <= 0.05 && math.Abs(v.ViewDir[1]) <= 0.05
}

// ModelToPaper maps a model-space point into this viewport's paper
// coordinates. ok=false means the viewport is unsupported (3D/off/invalid).
func (v LayoutViewport) ModelToPaper(x, y float64) (float64, float64, bool) {
	if !v.Supported() {
		return 0, 0, false
	}
	s := v.Height / v.ViewSize
	angle := -v.ViewTwist
	cos, sin := math.Cos(angle), math.Sin(angle)
	dx, dy := (x-v.ViewCenter[0])*s, (y-v.ViewCenter[1])*s
	return v.Center[0] + dx*cos - dy*sin, v.Center[1] + dx*sin + dy*cos, true
}

// ModelBoxToPaper maps a model-space bbox into this viewport's paper-space
// AABB. ok=false means the viewport is unsupported.
func (v LayoutViewport) ModelBoxToPaper(box [4]float64) ([4]float64, bool) {
	if !v.Supported() {
		return [4]float64{}, false
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range [][2]float64{{box[0], box[1]}, {box[2], box[1]}, {box[0], box[3]}, {box[2], box[3]}} {
		x, y, ok := v.ModelToPaper(p[0], p[1])
		if !ok {
			return [4]float64{}, false
		}
		minX, minY = math.Min(minX, x), math.Min(minY, y)
		maxX, maxY = math.Max(maxX, x), math.Max(maxY, y)
	}
	return [4]float64{minX, minY, maxX, maxY}, true
}

// ModelBoxToPaper maps a model-space bbox into paper coordinates using the
// first supported viewport whose paper rectangle contains the transformed
// box center. Model layouts return the box unchanged.
func (l Layout) ModelBoxToPaper(box [4]float64) ([4]float64, bool) {
	if l.Space == SpaceModel {
		return box, true
	}
	cx, cy := (box[0]+box[2])/2, (box[1]+box[3])/2
	for _, vp := range l.Viewports {
		if !vp.Supported() || vp.PaperRepresentation() {
			continue
		}
		px, py, ok := vp.ModelToPaper(cx, cy)
		if !ok || px < vp.Rect[0] || px > vp.Rect[2] || py < vp.Rect[1] || py > vp.Rect[3] {
			continue
		}
		mapped, ok := vp.ModelBoxToPaper(box)
		if ok {
			return mapped, true
		}
	}
	return [4]float64{}, false
}

// layoutContentBounds tessellates a layout's entity set and returns its
// content bounds in native layout coordinates. Model space uses the same
// text-aware robust bounds as RenderPNG; paper layouts use exact extents so
// title-block and notes stay inside the sheet.
func layoutContentBounds(doc *drawing.Document, space SpaceKind) ([4]float64, bool) {
	prims := drawing.NewTessellator(doc).ExpandAll()
	if len(prims) == 0 {
		return [4]float64{}, false
	}
	var b drawing.Box2
	if space == SpaceModel {
		b = sheetContentBounds(prims)
	} else {
		b = drawing.PrimitivesBounds(prims)
	}
	if b.Invalid() || b.MaxX <= b.MinX || b.MaxY <= b.MinY {
		return [4]float64{}, false
	}
	return [4]float64{b.MinX, b.MinY, b.MaxX, b.MaxY}, true
}
