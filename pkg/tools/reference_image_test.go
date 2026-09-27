package tools

import (
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReferenceGoDecodersNeedNoRuntime(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	for _, tc := range []struct {
		name   string
		encode func(io.Writer, image.Image) error
	}{
		{"png", png.Encode}, {"jpeg", func(w io.Writer, im image.Image) error { return jpeg.Encode(w, im, nil) }}, {"gif", func(w io.Writer, im image.Image) error { return gif.Encode(w, im, nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Format recognition uses content rather than the suffix.
			path := filepath.Join(t.TempDir(), "reference.bmp")
			file, err := os.Create(path)
			require.NoError(t, err)
			require.NoError(t, tc.encode(file, img))
			require.NoError(t, file.Close())
			root := t.TempDir()
			decoded, err := loadReferenceImage(context.Background(), nil, nil, path, root)
			require.NoError(t, err)
			require.Equal(t, 3, decoded.Bounds().Dx())
			require.Equal(t, 2, decoded.Bounds().Dy())
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}
