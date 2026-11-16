// Raster snapshots cover drawing and text rendering. Viewport text coverage
// and outlier handling are asserted independently in viewport_test.go and
// corpus_regression_test.go, so a new snapshot cannot hide clipped labels.
package render

import (
	"bytes"
	"github.com/unitedrhino/go-cad/internal/drawing"
	"github.com/unitedrhino/go-cad/internal/testsupport"
	"image/png"
	"os"
	"testing"
)

// goldenMaxDiffRatio 允许的不相等像素占比上限（0.5%，见文件头基线决策记录）。
const goldenMaxDiffRatio = 0.005

// goldenCases 金标样本：dwg 为输入图纸，ref 为固化参照图，width 与固化时一致。
var goldenCases = []struct {
	dwg   string
	ref   string
	width int
}{
	{"lw_example2018.dwg", "lw_example2018.dwg.png", 2048},
	{"lw_Leader.dwg", "lw_Leader.dwg.png", 2048},
}

// TestRenderGoldenMatch 对每个金标样本渲染并与参照图逐像素对照。
func TestRenderGoldenMatch(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.dwg, func(t *testing.T) {
			refData, err := os.ReadFile(testsupport.TestdataPath(tc.ref))
			if err != nil {
				if os.IsNotExist(err) {
					t.Skipf("参照图缺失，跳过金标对照: %s", tc.ref)
				}
				t.Fatalf("读取参照图失败: %v", err)
			}
			ref, err := png.Decode(bytes.NewReader(refData))
			if err != nil {
				t.Fatalf("参照图不是合法 PNG: %v", err)
			}
			doc := parseIntegration(t, tc.dwg)
			pngData, err := RenderPNG(doc, RenderOptions{Width: tc.width})
			if err != nil {
				t.Fatalf("渲染失败: %v", err)
			}
			cur, err := png.Decode(bytes.NewReader(pngData))
			if err != nil {
				t.Fatalf("渲染输出不是合法 PNG: %v", err)
			}
			// 参照图按当前渲染固化，尺寸应严格一致（宽=固化宽度，高=包围盒比例）；
			// 不一致说明视口/比例逻辑已变，直接失败而非缩放比较（见文件头决策记录）
			rb, cb := ref.Bounds(), cur.Bounds()
			if rb.Dx() != cb.Dx() || rb.Dy() != cb.Dy() {
				t.Fatalf("尺寸与参照图不一致: cur=%dx%d ref=%dx%d（渲染视口逻辑已变，需重新固化参照图）",
					cb.Dx(), cb.Dy(), rb.Dx(), rb.Dy())
			}
			diff, total := 0, rb.Dx()*rb.Dy()
			for y := 0; y < rb.Dy(); y++ {
				for x := 0; x < rb.Dx(); x++ {
					cr, cg, cbl, _ := cur.At(cb.Min.X+x, cb.Min.Y+y).RGBA()
					rr, rg, rl, _ := ref.At(rb.Min.X+x, rb.Min.Y+y).RGBA()
					if channelDelta(cr, rr) || channelDelta(cg, rg) || channelDelta(cbl, rl) {
						diff++
					}
				}
			}
			ratio := float64(diff) / float64(total)
			t.Logf("不相等像素 %d/%d（%.4f%%），阈值 %.2f%%", diff, total, ratio*100, goldenMaxDiffRatio*100)
			if ratio > goldenMaxDiffRatio {
				t.Errorf("渲染结果与金标差异超阈值: %.4f%% > %.2f%%", ratio*100, goldenMaxDiffRatio*100)
			}
		})
	}
}

// channelDelta 判断单通道 16 位值是否不同（量化到 8 位后比较，消除 PNG 位深表达差异）。
func channelDelta(a, b uint32) bool {
	return a>>8 != b>>8
}

// parseIntegration 读取渲染门禁样本并解析（render 包本地副本；路径相对
// internal/render 测试工作目录，样本经 ../../testdata 取用）。
func parseIntegration(t *testing.T, name string) *drawing.Document {
	t.Helper()
	data, err := os.ReadFile(testsupport.TestdataPath(name))
	if err != nil {
		t.Skipf("样本缺失（%s）: %v", name, err)
	}
	doc, err := drawing.Parse(data)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", name, err)
	}
	return doc
}
