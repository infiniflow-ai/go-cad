package render

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"

	"github.com/unitedrhino/go-cad/internal/drawing"
)

// LayoutResult is the output of rendering one layout or layout region.
// Viewport is the actual native-coordinate viewport used for the image,
// including any render margin; callers must store it with the image instead
// of assuming the requested bbox was used verbatim.
type LayoutResult struct {
	LayoutID string     `json:"layout_id"`
	Name     string     `json:"name"`
	Space    SpaceKind  `json:"space"`
	Data     []byte     `json:"-"`
	Width    int        `json:"width"`
	Height   int        `json:"height"`
	Viewport [4]float64 `json:"viewport"`
	PassRate float64    `json:"pass_rate"`
	Warnings []string   `json:"warnings,omitempty"`
}

// RenderLayoutPNG renders one layout. Paper layouts render their paper-space
// annotations and compose active rectangular top-view VIEWPORTs. opts.Box, if
// set, overrides the layout-native render bounds.
func RenderLayoutPNG(doc *drawing.Document, layoutID string, opts RenderOptions) (LayoutResult, error) {
	if doc == nil {
		return LayoutResult{}, fmt.Errorf("cad: 文档为空，无法渲染")
	}
	groups := layoutGroups(doc)
	var group *layoutGroup
	for i := range groups {
		if groups[i].id == layoutID {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		return LayoutResult{}, fmt.Errorf("cad: 未知布局 %q", layoutID)
	}
	viewport, err := layoutRenderViewport(group, opts.Box)
	if err != nil {
		return LayoutResult{}, err
	}
	return renderLayoutGroup(doc, group, viewport, opts)
}

// RenderLayoutBoxPNG renders a region of one layout in that layout's native
// coordinates. The returned Viewport includes the standard crop margin, so
// pixel coordinates map back to the image exactly.
func RenderLayoutBoxPNG(doc *drawing.Document, layoutID string, box [4]float64, opts RenderOptions) (LayoutResult, error) {
	if doc == nil {
		return LayoutResult{}, fmt.Errorf("cad: 文档为空，无法渲染")
	}
	if box[2] <= box[0] || box[3] <= box[1] {
		return LayoutResult{}, fmt.Errorf("cad: 布局 %q 的裁剪框无效: %v", layoutID, box)
	}
	groups := layoutGroups(doc)
	var group *layoutGroup
	for i := range groups {
		if groups[i].id == layoutID {
			group = &groups[i]
			break
		}
	}
	if group == nil {
		return LayoutResult{}, fmt.Errorf("cad: 未知布局 %q", layoutID)
	}
	viewport := sheetViewport(box)
	return renderLayoutGroup(doc, group, viewport, opts)
}

// layoutRenderViewport chooses the native render viewport for a whole layout.
// Paper layouts with a declared paper size render the full sheet; otherwise
// the layout content bounds are used with the standard render margin.
func layoutRenderViewport(group *layoutGroup, box *[4]float64) (drawing.Box2, error) {
	if box != nil {
		b := drawing.Box2{MinX: box[0], MinY: box[1], MaxX: box[2], MaxY: box[3]}
		if b.Invalid() || b.MaxX <= b.MinX || b.MaxY <= b.MinY {
			return drawing.Box2{}, fmt.Errorf("cad: 渲染框无效: %v", *box)
		}
		return sheetViewport(*box), nil
	}
	b := drawing.Box2{MinX: group.bounds[0], MinY: group.bounds[1], MaxX: group.bounds[2], MaxY: group.bounds[3]}
	if group.hasBounds && !b.Invalid() && b.MaxX > b.MinX && b.MaxY > b.MinY {
		if group.space == SpacePaper && group.paperSize[0] > 0 && group.paperSize[1] > 0 {
			paper := drawing.Box2{MinX: 0, MinY: 0, MaxX: group.paperSize[0], MaxY: group.paperSize[1]}
			b = unionBox(paper, b)
		}
	} else if group.space == SpacePaper {
		b = drawing.Box2{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	}
	// DXF layouts do not carry LAYOUT paper sizes. The paper-space viewport
	// rectangles still describe the sheet extent, so include them instead of
	// shrinking the render to the title-block text bounds.
	if group.space == SpacePaper && (group.paperSize[0] <= 0 || group.paperSize[1] <= 0) {
		for _, vp := range group.viewports {
			b.Extend(vp.Rect[0], vp.Rect[1])
			b.Extend(vp.Rect[2], vp.Rect[3])
		}
	}
	if b.Invalid() || b.MaxX <= b.MinX || b.MaxY <= b.MinY {
		b = drawing.Box2{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}
	}
	if group.space == SpaceModel {
		return renderBoundsWithMargin(b, 0.05), nil
	}
	// Paper annotations may sit slightly outside the nominal paper rectangle
	// (plot margins/title-block overflow); keep the union visible with a
	// small margin, but do not shrink a full sheet to its content extents.
	return renderBoundsWithMargin(b, 0.01), nil
}

func renderBoundsWithMargin(b drawing.Box2, frac float64) drawing.Box2 {
	mx := (b.MaxX - b.MinX) * frac
	my := (b.MaxY - b.MinY) * frac
	if mx == 0 && my == 0 {
		mx, my = 1, 1
	} else if mx == 0 {
		mx = my
	} else if my == 0 {
		my = mx
	}
	return drawing.Box2{MinX: b.MinX - mx, MinY: b.MinY - my, MaxX: b.MaxX + mx, MaxY: b.MaxY + my}
}

func unionBox(a, b drawing.Box2) drawing.Box2 {
	out := a
	out.Extend(b.MinX, b.MinY)
	out.Extend(b.MaxX, b.MaxY)
	return out
}

// renderLayoutGroup rasterizes a layout into a single canvas. Paper layout
// order is: VIEWPORT model content first, paper-space annotations second, so
// title blocks and notes always sit on top of the model view.
func renderLayoutGroup(doc *drawing.Document, group *layoutGroup, viewport drawing.Box2, opts RenderOptions) (LayoutResult, error) {
	opts = normalizeSheetOptions(opts)
	width := opts.Width
	if width <= 0 {
		width = 2048
	}
	scale := float64(width) / (viewport.MaxX - viewport.MinX)
	height := int((viewport.MaxY-viewport.MinY)*scale + 0.5)
	if height < 1 {
		height = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	drawRect(img, img.Rect, opts.Background)
	cv := &canvas{img: img, width: float64(width), height: float64(height), lineWidth: adaptiveLineWidth(float64(width)), fontPath: opts.FontPath}
	cv.setTransform(viewport, scale)

	result := LayoutResult{
		LayoutID: group.id,
		Name:     group.name,
		Space:    group.space,
		Width:    width,
		Height:   height,
		Viewport: [4]float64{viewport.MinX, viewport.MinY, viewport.MaxX, viewport.MaxY},
	}
	bgWhite := opts.Background.R > 127
	var drawn []drawing.Primitive

	if group.space == SpaceModel {
		prims := layoutPrimitives(doc, group)
		drawn = prims
		for i := range prims {
			if primInBox(&prims[i], viewport) {
				cv.drawPrimitive(prims[i], doc, bgWhite)
			}
		}
	} else {
		prims := layoutPrimitives(doc, group)
		paperPrims := make([]drawing.Primitive, 0, len(prims))
		for i := range prims {
			if primInBox(&prims[i], viewport) {
				paperPrims = append(paperPrims, prims[i])
			}
		}
		modelPrims := modelLayoutPrimitives(doc)
		for _, vp := range group.viewports {
			pieces, warnings := composeViewport(modelPrims, vp, group.paperSize)
			result.Warnings = append(result.Warnings, warnings...)
			if len(pieces) == 0 {
				continue
			}
			clip := viewportPixelRect(vp.Rect, viewport, scale)
			if clip.Empty() {
				continue
			}
			cv.setClip(&clip)
			for i := range pieces {
				cv.drawPrimitive(pieces[i], doc, bgWhite)
			}
			cv.setClip(nil)
			drawn = append(drawn, pieces...)
		}
		// Paper annotations last (title block, notes, tables, legend text).
		for i := range paperPrims {
			cv.drawPrimitive(paperPrims[i], doc, bgWhite)
		}
		drawn = append(drawn, paperPrims...)
	}

	result.PassRate = sheetTextPassRate(drawn, viewport, scale)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return LayoutResult{}, err
	}
	result.Data = buf.Bytes()
	return result, nil
}

// layoutPrimitives expands a single layout's entity set with the same
// filtering used by model rendering.
func layoutPrimitives(doc *drawing.Document, group *layoutGroup) []drawing.Primitive {
	copyDoc := *doc
	copyDoc.ModelSpace = group.entities
	copyDoc.PspaceSpace = nil
	prims := drawing.NewTessellator(&copyDoc).ExpandAll()
	prims = filterRadiatingStrokes(prims)
	base := quantileSheetBounds(prims)
	if base.Invalid() {
		base = drawing.Box2{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}
	}
	return dropOriginAnchored(prims, base)
}

// modelLayoutPrimitives expands and filters model space for VIEWPORT composition.
func modelLayoutPrimitives(doc *drawing.Document) []drawing.Primitive {
	copyDoc := *doc
	copyDoc.PspaceSpace = nil
	prims := drawing.NewTessellator(&copyDoc).ExpandAll()
	prims = filterRadiatingStrokes(prims)
	base := quantileSheetBounds(prims)
	if base.Invalid() {
		base = drawing.Box2{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}
	}
	return dropOriginAnchored(prims, base)
}

// composeViewport transforms model primitives into paper coordinates for one
// VIEWPORT and returns them ready for drawing. Unsupported viewports are
// skipped with a warning instead of silently rendering wrong geometry.
func composeViewport(modelPrims []drawing.Primitive, vp LayoutViewport, paperSize [2]float64) ([]drawing.Primitive, []string) {
	var warnings []string
	if vp.Width <= 0 || vp.Height <= 0 {
		return nil, nil
	}
	if !vp.Active {
		return nil, nil
	}
	if isPaperRepresentationViewport(vp, paperSize) {
		return nil, nil
	}
	dir := vp.ViewDir
	if math.Abs(dir[2]) < 0.99 || math.Abs(dir[0]) > 0.05 || math.Abs(dir[1]) > 0.05 {
		warnings = append(warnings, fmt.Sprintf("VIEWPORT %x 不是俯视图（dir=%v），已跳过模型内容", vp.Handle, dir))
		return nil, warnings
	}
	if vp.ViewSize <= 0 {
		warnings = append(warnings, fmt.Sprintf("VIEWPORT %x view_size=%.3f 无效，已跳过模型内容", vp.Handle, vp.ViewSize))
		return nil, warnings
	}
	s := vp.Height / vp.ViewSize
	if s <= 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return nil, warnings
	}
	angle := -vp.ViewTwist
	cos, sin := math.Cos(angle), math.Sin(angle)
	cx, cy := vp.Center[0], vp.Center[1]
	vx, vy := vp.ViewCenter[0], vp.ViewCenter[1]
	apply := func(x, y float64) (float64, float64) {
		dx, dy := (x-vx)*s, (y-vy)*s
		return cx + dx*cos - dy*sin, cy + dx*sin + dy*cos
	}
	// Model-space box corresponding to the viewport rectangle (plus margin)
	// for coarse prefiltering; exact clipping is done on the pixel canvas.
	rect := drawing.Box2{MinX: vp.Rect[0], MinY: vp.Rect[1], MaxX: vp.Rect[2], MaxY: vp.Rect[3]}
	inv := drawing.Box2{
		MinX: vx + (rect.MinX-cx)/s - 0.1*vp.ViewSize,
		MinY: vy + (rect.MinY-cy)/s - 0.1*vp.ViewSize,
		MaxX: vx + (rect.MaxX-cx)/s + 0.1*vp.ViewSize,
		MaxY: vy + (rect.MaxY-cy)/s + 0.1*vp.ViewSize,
	}
	out := make([]drawing.Primitive, 0, len(modelPrims))
	for i := range modelPrims {
		if !primInBox(&modelPrims[i], inv) {
			continue
		}
		out = append(out, transformPrimitive(modelPrims[i], apply, angle, s))
	}
	return out, warnings
}

// isPaperRepresentationViewport reports whether a VIEWPORT is the synthetic
// paper-space viewport (its view is the paper itself), which must not be
// composited as model content.
func isPaperRepresentationViewport(vp LayoutViewport, paperSize [2]float64) bool {
	if !vp.PaperRepresentation() {
		return false
	}
	if paperSize[0] > 0 && paperSize[1] > 0 {
		return vp.Width >= 0.8*paperSize[0] && vp.Height >= 0.8*paperSize[1]
	}
	return true
}

// transformPrimitive applies a uniform scale + rotation + translation to one
// expanded primitive.
func transformPrimitive(p drawing.Primitive, apply func(x, y float64) (float64, float64), angle, scale float64) drawing.Primitive {
	out := p
	if p.Kind == 0 {
		out.Strokes = make([]drawing.Stroke, len(p.Strokes))
		for i, s := range p.Strokes {
			x1, y1 := apply(s.X1, s.Y1)
			x2, y2 := apply(s.X2, s.Y2)
			out.Strokes[i] = drawing.Stroke{X1: x1, Y1: y1, X2: x2, Y2: y2}
		}
		return out
	}
	x, y := apply(p.Lb.X, p.Lb.Y)
	out.Lb.X, out.Lb.Y = x, y
	out.Lb.Rot += angle
	out.Lb.H *= scale
	out.Lb.W *= scale
	if p.Lb.Tx != nil {
		tx := *p.Lb.Tx
		ux, uy := apply(p.Lb.X+p.Lb.Tx.Ux, p.Lb.Y+p.Lb.Tx.Uy)
		vx, vy := apply(p.Lb.X+p.Lb.Tx.Vx, p.Lb.Y+p.Lb.Tx.Vy)
		tx.Ux, tx.Uy = ux-x, uy-y
		tx.Vx, tx.Vy = vx-x, vy-y
		tx.RectWidth *= scale
		out.Lb.Tx = &tx
	}
	return out
}

// viewportPixelRect maps a paper-space viewport rectangle to canvas pixels.
func viewportPixelRect(rect [4]float64, viewport drawing.Box2, scale float64) image.Rectangle {
	x0 := int(math.Floor((rect[0] - viewport.MinX) * scale))
	x1 := int(math.Ceil((rect[2] - viewport.MinX) * scale))
	y0 := int(math.Floor((viewport.MaxY - rect[3]) * scale))
	y1 := int(math.Ceil((viewport.MaxY - rect[1]) * scale))
	return image.Rect(x0, y0, x1, y1)
}
