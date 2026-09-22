//go:build integration

package tools

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Read the saved document in a fresh Aseprite process: palette metadata alone
// cannot detect the opaque index-zero loss or an unchanged RGB raster.
func TestQuantizationContractOpaquePixels(t *testing.T) {
	for _, algorithm := range []string{"median_cut", "kmeans", "octree"} {
		for _, dither := range []bool{false, true} {
			for _, indexed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/dither=%v/indexed=%v", algorithm, dither, indexed), func(t *testing.T) {
					f := newBehaviorFixture(t)
					p := f.sprite(aseprite.ColorModeRGB)
					f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB)
for y=0,15 do for x=0,15 do im:drawPixel(x,y,x<8 and app.pixelColor.rgba(255,0,0,255) or app.pixelColor.rgba(0,0,255,255)) end end
s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
					r := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 2, "algorithm": algorithm, "dither": dither, "convert_to_indexed": indexed})
					require.Equal(t, float64(2), r["quantized_colors"])
					f.lua(p, `local s=app.activeSprite;local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,1)
local red,blue=0,0
for px in im:pixels() do local v=px();assert(app.pixelColor.rgbaA(v)==255,"opaque pixel lost");if app.pixelColor.rgbaR(v)==255 then red=red+1 elseif app.pixelColor.rgbaB(v)==255 then blue=blue+1 else error("unexpected color") end end
assert(red==128 and blue==128,"two opaque colors collapsed")`)
				})
			}
		}
	}
}

func TestQuantizationContractRGBRemapsWithoutFlattening(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(4,4,ColorMode.RGB);local cols={0xff0000ff,0xff00ff00,0xffff0000,0xff00ffff}
for y=0,3 do for x=0,3 do im:drawPixel(x,y,cols[x+1]) end end
local c=s:newCel(s.layers[1],1,im,Point(3,5));c.data="cel metadata";c.opacity=180
local g=s:newGroup();g.name="group";s.layers[1].parent=g;local hidden=s:newLayer();hidden.isVisible=false
s.data="sprite metadata";s:saveAs(s.filename)`)
	r := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 2, "algorithm": "median_cut", "dither": false, "convert_to_indexed": false, "preserve_transparency": false})
	require.Equal(t, "rgb", r["color_mode"])
	f.lua(p, `local s=app.activeSprite;assert(s.colorMode==ColorMode.RGB);assert(s.data=="sprite metadata");assert(#s.layers==2)
local c;for _,v in ipairs(s.cels) do c=v end
assert(c.position.x==3 and c.position.y==5);assert(c.data=="cel metadata" and c.opacity==180);assert(c.layer.parent.name=="group")
local colors={};for px in c.image:pixels() do colors[px()]=true end;local n=0;for _ in pairs(colors) do n=n+1 end;assert(n==2,"RGB pixels were not reduced")
for px in c.image:pixels() do local matched=false;for i=0,#s.palettes[1]-1 do local col=s.palettes[1]:getColor(i);if px()==app.pixelColor.rgba(col.red,col.green,col.blue,col.alpha) then matched=true end end;assert(matched,"pixel outside palette") end`)
}

func TestQuantizationContractPreservesInputModeAndTransparency(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeIndexed, aseprite.ColorModeGrayscale} {
		for _, dither := range []bool{false, true} {
			for _, preserve := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/dither=%v/preserve=%v", mode, dither, preserve), func(t *testing.T) {
					f := newBehaviorFixture(t)
					p := f.sprite(aseprite.ColorModeRGB)
					f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);for y=4,11 do for x=4,11 do im:drawPixel(x,y,app.pixelColor.rgba(x<8 and 255 or 120,x<8 and 255 or 120,x<8 and 255 or 120,255)) end end;s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
					if mode != aseprite.ColorModeRGB {
						format := "indexed"
						f.lua(p, `local pal=app.activeSprite.palettes[1];pal:resize(3);pal:setColor(0,Color{r=0,g=0,b=0,a=0});pal:setColor(1,Color{r=255,g=255,b=255,a=255});pal:setColor(2,Color{r=120,g=120,b=120,a=255});app.activeSprite:saveAs(app.activeSprite.filename)`)
						if mode == aseprite.ColorModeGrayscale {
							format = "gray"
						}
						f.lua(p, fmt.Sprintf(`app.command.ChangePixelFormat{ui=false,format="%s"};app.activeSprite:saveAs(app.activeSprite.filename)`, format))
					}
					r := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 3, "algorithm": "median_cut", "dither": dither, "convert_to_indexed": false, "preserve_transparency": preserve})
					require.Equal(t, string(mode), r["color_mode"])
					f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);assert(app.pixelColor.rgbaA(im:getPixel(0,0))==0);assert(app.pixelColor.rgbaA(im:getPixel(5,5))==255);assert(im:getPixel(5,5)~=im:getPixel(9,5),"shades collapsed")`)
				})
			}
		}
	}
}

func TestQuantizationContractIndexedBudget(t *testing.T) {
	for _, transparent := range []bool{false, true} {
		t.Run(fmt.Sprint(transparent), func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);for y=0,15 do for x=0,15 do local v=y*16+x;im:drawPixel(x,y,app.pixelColor.rgba(v,v,v,255)) end end;if %t then im:drawPixel(0,0,0) end;s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`, transparent))
			r := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 256, "algorithm": "median_cut", "dither": false, "convert_to_indexed": true})
			expected := 255
			if transparent {
				expected = 256
			}
			require.Equal(t, float64(expected), r["quantized_colors"])
			f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);local count=0;local colors={};for px in im:pixels() do if app.pixelColor.rgbaA(px())==0 then count=count+1 else colors[px()]=true end end;assert(count==%d);local n=0;for _ in pairs(colors) do n=n+1 end;assert(n==255,"opaque palette color was lost")`, map[bool]int{false: 0, true: 1}[transparent]))
		})
	}
}

func TestQuantizationContractRejectsAnimationWithoutChanges(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;s:newEmptyFrame();s:saveAs(s.filename)`)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "quantize_palette", Arguments: map[string]any{"sprite_path": p, "target_colors": 2, "algorithm": "median_cut", "dither": true}})
	require.NoError(t, err)
	require.True(t, r.IsError)
	require.Contains(t, r.Content[0].(*mcp.TextContent).Text, "single-frame")
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestQuantizationContractRepeatedIndexedWithUnusedMask(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawPixel(5,5,app.pixelColor.rgba(255,0,0,255));s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
	for i := 0; i < 2; i++ {
		f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 2, "algorithm": "median_cut", "dither": false, "preserve_transparency": false, "convert_to_indexed": true})
		f.lua(p, `local s=app.activeSprite;assert(s.transparentColor==255);local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);assert(app.pixelColor.rgbaA(im:getPixel(0,0))==0);assert(app.pixelColor.rgbaR(im:getPixel(5,5))==255)`)
	}
}

func TestQuantizationContractIndexedKeepsCelMetadata(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(4,4,ColorMode.RGB);im:drawPixel(1,1,app.pixelColor.rgba(255,0,0,255));im:drawPixel(2,2,app.pixelColor.rgba(0,0,255,255))
local c=s:newCel(s.layers[1],1,im,Point(3,5));c.data="metadata";c.opacity=180;c.zIndex=2;c.color=Color{r=17,g=31,b=49,a=255};c.properties("test").key="value"
local hidden=s:newLayer();hidden.name="hidden";hidden.isVisible=false
local empty=s:newCel(hidden,1,Image(2,2,ColorMode.RGB),Point(7,8));empty.data="empty";empty.zIndex=3;empty.properties("test").key="empty-value"
s:saveAs(s.filename)`)
	f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 3, "algorithm": "median_cut", "dither": false, "convert_to_indexed": true})
	f.lua(p, `local s=app.activeSprite;local c=s.layers[1]:cel(1);assert(c.position.x==3 and c.position.y==5);assert(c.opacity==180 and c.data=="metadata" and c.zIndex==2);assert(c.color.red==17 and c.color.green==31 and c.color.blue==49);assert(c.properties("test").key=="value")
local e=s.layers[2]:cel(1);assert(not s.layers[2].isVisible);assert(e.position.x==7 and e.position.y==8 and e.data=="empty" and e.zIndex==3);assert(e.properties("test").key=="empty-value")`)
}

func TestQuantizationContractGrayscaleReduction(t *testing.T) {
	for _, algorithm := range []string{"median_cut", "kmeans", "octree"} {
		for _, dither := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", algorithm, dither), func(t *testing.T) {
				f := newBehaviorFixture(t)
				p := f.sprite(aseprite.ColorModeGrayscale)
				f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.GRAY);for y=0,15 do for x=0,15 do im:drawPixel(x,y,app.pixelColor.graya(y*16+x,255)) end end;s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
				r := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 4, "algorithm": algorithm, "dither": dither, "convert_to_indexed": false})
				require.Equal(t, "grayscale", r["color_mode"])
				f.lua(p, `local s=app.activeSprite;assert(s.colorMode==ColorMode.GRAY);local palette={};for i=0,#s.palettes[1]-1 do local c=s.palettes[1]:getColor(i);assert(c.red==c.green and c.green==c.blue);palette[c.red]=true end
local count=0;local colors={};for px in s.layers[1]:cel(1).image:pixels() do local v=app.pixelColor.grayaV(px());assert(palette[v]);assert(app.pixelColor.grayaA(px())==255);colors[v]=true end;for _ in pairs(colors) do count=count+1 end;assert(count>1 and count<=4)`)
			})
		}
	}
}
