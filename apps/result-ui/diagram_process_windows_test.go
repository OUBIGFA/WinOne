package resultui

import (
	"context"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalDiagramRenderer(t *testing.T) {
	exe := os.Getenv("WINONE_DIAGRAM_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_DIAGRAM_TEST_EXE for the native renderer")
	}
	output := os.Getenv("WINONE_DIAGRAM_TEST_OUTPUT")
	if output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	longLabels, err := os.ReadFile(filepath.Join("..", "diagram-renderer", "tests", "fixtures", "long-edge-labels.mmd"))
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{"long-labels": string(longLabels), "flow": "flowchart LR\n A[用户输入] --> B{是否有效}\n B -->|是| C[显示结果]\n B -->|否| D[修改]", "sequence": "sequenceDiagram\n participant U as 用户\n participant A as AI服务\n U->>A: 发送请求\n A-->>U: 返回结果"} {
		for _, theme := range []string{"light", "dark"} {
			t.Run(name+"/"+theme, func(t *testing.T) {
				started := time.Now()
				bitmap, err := renderLocalDiagram(context.Background(), exe, t.TempDir(), source, theme == "dark")
				if err != nil {
					t.Fatal(err)
				}
				w, h := bitmap.Size()
				t.Logf("native render %dx%d: %s", w, h, time.Since(started))
				if w < 100 || h < 100 {
					t.Fatal("diagram is empty")
				}
				if output != "" {
					tt := ui.NewTester(func(c *ui.Context) { ui.Image(c, bitmap).Size(float32(w), float32(h)) }, w, h)
					file, err := os.Create(filepath.Join(output, name+"-"+theme+".png"))
					if err != nil {
						t.Fatal(err)
					}
					defer file.Close()
					if err = png.Encode(file, tt.Image()); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
	if _, err := renderLocalDiagram(context.Background(), exe, t.TempDir(), "invalid graph", false); err == nil {
		t.Fatal("invalid graph reported success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := renderLocalDiagram(ctx, exe, t.TempDir(), "flowchart LR; A-->B", false); err == nil {
		t.Fatal("cancelled diagram rendered")
	}
}
