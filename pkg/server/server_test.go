package server

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"testing"

	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/sinks"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
)

func TestNew(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	logger := mtlog.New(mtlog.WithSink(sinks.NewMemorySink()))

	server, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if server == nil {
		t.Fatal("New() returned nil server")
	}

	if server.client == nil {
		t.Error("server.client is nil")
	}

	if server.gen == nil {
		t.Error("server.gen is nil")
	}

	if server.config != cfg {
		t.Error("server.config does not match provided config")
	}

	if server.logger == nil {
		t.Error("server.logger is nil")
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	logger := mtlog.New(mtlog.WithSink(sinks.NewMemorySink()))

	tests := []struct {
		name          string
		asepritePath  string
		wantErrSubstr string
	}{
		{
			name:          "empty aseprite path",
			asepritePath:  "",
			wantErrSubstr: "aseprite executable not found",
		},
		{
			name:          "nonexistent aseprite path",
			asepritePath:  "D:\\nonexistent\\aseprite.exe",
			wantErrSubstr: "aseprite executable not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testutil.CreateTestConfigWithPath(t, tt.asepritePath)

			_, err := New(cfg, logger)
			if err == nil {
				t.Fatal("New() expected error, got nil")
			}

			if tt.wantErrSubstr != "" {
				errMsg := err.Error()
				if !contains(errMsg, tt.wantErrSubstr) {
					t.Errorf("New() error = %v, want substring %q", err, tt.wantErrSubstr)
				}
			}
		})
	}
}

func TestServer_Client(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	logger := mtlog.New(mtlog.WithSink(sinks.NewMemorySink()))

	server, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client := server.Client()
	if client == nil {
		t.Error("Client() returned nil")
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSnapshotToolsRegistered(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	s, err := New(cfg, mtlog.New(mtlog.WithSink(sinks.NewMemorySink())))
	if err != nil {
		t.Fatal(err)
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.mcp.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 64 {
		t.Fatalf("registered %d tools, want 64", len(list.Tools))
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, name := range []string{
		"add_frame",
		"add_layer",
		"add_palette_color",
		"analyze_palette_harmonies",
		"analyze_reference",
		"apply_auto_shading",
		"apply_outline",
		"apply_shading",
		"copy_selection",
		"create_canvas",
		"create_layer_group",
		"create_snapshot",
		"create_tag",
		"crop_sprite",
		"cut_selection",
		"delete_frame",
		"delete_layer",
		"delete_snapshot",
		"delete_tag",
		"deselect",
		"downsample_image",
		"draw_circle",
		"draw_contour",
		"draw_line",
		"draw_pixels",
		"draw_rectangle",
		"draw_with_dither",
		"duplicate_frame",
		"export_sprite",
		"export_spritesheet",
		"fill_area",
		"flatten_layers",
		"flip_sprite",
		"get_palette",
		"get_pixels",
		"get_sprite_info",
		"get_sprite_structure",
		"get_sprite_tags",
		"import_image",
		"link_cel",
		"list_operation_history",
		"list_snapshots",
		"move_layer",
		"move_selection",
		"paste_clipboard",
		"quantize_palette",
		"resize_canvas",
		"restore_snapshot",
		"rotate_sprite",
		"save_as",
		"scale_sprite",
		"select_all",
		"select_ellipse",
		"select_rectangle",
		"set_cel_properties",
		"set_frame_duration",
		"set_layer_properties",
		"set_palette",
		"set_palette_color",
		"set_tag_properties",
		"sort_palette",
		"suggest_antialiasing",
		"undo_last_operation",
		"unlink_cel",
	} {
		if !names[name] {
			t.Errorf("missing %s", name)
		}
	}
}
