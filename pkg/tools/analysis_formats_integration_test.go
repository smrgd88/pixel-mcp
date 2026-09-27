//go:build integration

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func newReferenceBehavior(t *testing.T) *behaviorFixture {
	_, session, client := createAnalysisTestSession(t)
	t.Cleanup(func() { session.Close() })
	return &behaviorFixture{t, session, client, aseprite.NewLuaGenerator()}
}
func referenceFixture(t *testing.T, f *behaviorFixture) string {
	t.Helper()
	path := f.sprite(aseprite.ColorModeRGB)
	f.lua(path, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB)
local colors={Color{r=0,g=0,b=0},Color{r=255,g=255,b=255},Color{r=255,g=0,b=0},Color{r=0,g=255,b=0},Color{r=0,g=0,b=255},Color{r=255,g=255,b=0},Color{r=0,g=255,b=255},Color{r=255,g=0,b=255}}
s.palettes[1]:resize(#colors);for i,c in ipairs(colors) do s.palettes[1]:setColor(i-1,c) end
for y=0,15 do for x=0,15 do local c=colors[(math.floor(x/4)+math.floor(y/4))%8+1];im:drawPixel(x,y,app.pixelColor.rgba(c.red,c.green,c.blue,255)) end end
s:newCel(s.layers[1],1,im);s.data="reference metadata";s:saveAs(s.filename)`)
	return path
}
func referenceArgs(path string) map[string]any {
	return map[string]any{"reference_path": path, "target_width": 8, "target_height": 8, "palette_size": 5, "brightness_levels": 5, "edge_threshold": 30}
}
func TestReferenceAdvertisedFormats(t *testing.T) {
	f := newReferenceBehavior(t)
	source := referenceFixture(t, f)
	var baseline map[string]any
	for _, ext := range []string{"png", "jpg", "gif", "bmp", "aseprite"} {
		t.Run(ext, func(t *testing.T) {
			child := *f
			child.t = t
			f := &child
			path := source
			if ext != "aseprite" {
				path = filepath.Join(t.TempDir(), "reference."+ext)
				f.lua(source, fmt.Sprintf(`assert(app.activeSprite:saveCopyAs("%s"))`, aseprite.EscapeString(path)))
			}
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			out := f.call("analyze_reference", referenceArgs(path))
			require.NotEmpty(t, out["palette"])
			require.NotNil(t, out["composition"])
			dims := out["metadata"].(map[string]any)["source_dimensions"].(map[string]any)
			require.Equal(t, float64(16), dims["width"])
			require.Equal(t, float64(16), dims["height"])
			if ext == "png" {
				baseline = out
			}
			if ext == "bmp" || ext == "aseprite" {
				require.Equal(t, baseline["brightness_map"], out["brightness_map"])
				require.Equal(t, baseline["edge_map"], out["edge_map"])
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
func TestReferenceNativeFirstFrame(t *testing.T) {
	f := newReferenceBehavior(t)
	source := referenceFixture(t, f)
	reference := filepath.Join(t.TempDir(), "first.png")
	f.lua(source, f.gen.ExportSpriteFiles([]string{reference}, []int{1}, 1))
	expected := f.call("analyze_reference", referenceArgs(reference))
	f.lua(source, `local s=app.activeSprite;s:newEmptyFrame();local im=Image(16,16,ColorMode.RGB);im:clear(app.pixelColor.rgba(255,255,255,255));s:newCel(s.layers[1],2,im);s:saveAs(s.filename)`)
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	got := f.call("analyze_reference", referenceArgs(source))
	require.Equal(t, expected["brightness_map"], got["brightness_map"])
	require.Equal(t, expected["edge_map"], got["edge_map"])
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
func TestReferenceInvalidFilesPreserveSource(t *testing.T) {
	f := newReferenceBehavior(t)
	for _, tc := range []struct {
		ext  string
		data []byte
	}{
		{"png", []byte("broken PNG")}, {"gif", []byte("GIF89a")}, {"bmp", []byte("BMbroken")}, {"aseprite", []byte{128, 0, 0, 0, 224, 165}}, {"bin", []byte("unsupported")},
	} {
		t.Run(tc.ext, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid."+tc.ext)
			require.NoError(t, os.WriteFile(path, tc.data, 0600))
			result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "analyze_reference", Arguments: referenceArgs(path)})
			require.NoError(t, err)
			require.True(t, result.IsError)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tc.data, after)
		})
	}
}

func TestReferenceNativeCompositeModes(t *testing.T) {
	for _, mode := range []string{"rgb", "indexed", "gray"} {
		t.Run(mode, func(t *testing.T) {
			f := newReferenceBehavior(t)
			source := referenceFixture(t, f)
			f.lua(source, fmt.Sprintf(`local s=app.activeSprite
if "%s"~="rgb" then app.command.ChangePixelFormat{ui=false,format="%s"} end
local group=s:newGroup();group.name="nested";s.layers[1].parent=group;local layer=group.layers[1];layer.opacity=128;layer:cel(1).position=Point(2,3)
local hidden=s:newLayer();hidden.isVisible=false;local im=Image(16,16,s.colorMode);im:clear(Color{r=255,g=255,b=255,a=255});s:newCel(hidden,1,im)
s:saveAs(s.filename)`, mode, mode))
			pngPath := filepath.Join(t.TempDir(), "composite.png")
			f.lua(source, f.gen.ExportSpriteFiles([]string{pngPath}, []int{1}, 1))
			expected := f.call("analyze_reference", referenceArgs(pngPath))
			before, err := os.ReadFile(source)
			require.NoError(t, err)
			actual := f.call("analyze_reference", referenceArgs(source))
			require.Equal(t, expected["brightness_map"], actual["brightness_map"])
			require.Equal(t, expected["edge_map"], actual["edge_map"])
			after, err := os.ReadFile(source)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
func TestReferenceTemporaryCleanup(t *testing.T) {
	f := newReferenceBehavior(t)
	source := referenceFixture(t, f)
	root := t.TempDir()
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	image, err := loadReferenceImage(context.Background(), f.client, f.gen, source, root)
	require.NoError(t, err)
	require.Equal(t, 16, image.Bounds().Dx())
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
	broken := filepath.Join(t.TempDir(), "broken.bmp")
	require.NoError(t, os.WriteFile(broken, []byte("BMbroken"), 0600))
	_, err = loadReferenceImage(context.Background(), f.client, f.gen, broken, root)
	require.Error(t, err)
	entries, err = os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
	unavailable := aseprite.NewClient(filepath.Join(t.TempDir(), "missing-aseprite"), t.TempDir(), time.Second)
	_, err = loadReferenceImage(context.Background(), unavailable, f.gen, source, root)
	require.Error(t, err)
	entries, err = os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = loadReferenceImage(ctx, f.client, f.gen, source, root)
	require.ErrorIs(t, err, context.Canceled)
	entries, err = os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
func TestReferenceGIFUsesFirstImage(t *testing.T) {
	f := newReferenceBehavior(t)
	source := referenceFixture(t, f)
	path := filepath.Join(t.TempDir(), "animated.gif")
	f.lua(source, `local s=app.activeSprite;s:newEmptyFrame();local im=Image(16,16,ColorMode.RGB);im:clear(app.pixelColor.rgba(255,255,255,255));s:newCel(s.layers[1],2,im);s:saveAs(s.filename)`)
	f.lua(source, fmt.Sprintf(`assert(app.activeSprite:saveCopyAs("%s"))`, aseprite.EscapeString(path)))
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	// The existing Go path must keep working without an Aseprite executable.
	image, err := loadReferenceImage(context.Background(), nil, nil, path, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, 16, image.Bounds().Dx())
	r, g, b, _ := image.At(0, 0).RGBA()
	require.Zero(t, r)
	require.Zero(t, g)
	require.Zero(t, b)
	out := f.call("analyze_reference", referenceArgs(path))
	require.NotEmpty(t, out["palette"])
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestReferenceReadOnlyNativeAndASEAlias(t *testing.T) {
	f := newReferenceBehavior(t)
	source := referenceFixture(t, f)
	alias := filepath.Join(t.TempDir(), "reference.ase")
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(alias, data, 0400))
	defer os.Chmod(alias, 0600)
	result := f.call("analyze_reference", referenceArgs(alias))
	require.NotEmpty(t, result["palette"])
	after, err := os.ReadFile(alias)
	require.NoError(t, err)
	require.Equal(t, data, after)
	st, err := os.Stat(alias)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0400), st.Mode().Perm())
}
