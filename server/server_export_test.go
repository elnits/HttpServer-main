package server

import (
	"reflect"
	"testing"
)

func TestNormalizeTargetURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"basic_http", "http://localhost:9999/api/v1/upload", "http://localhost:9999/api/v1/upload", false},
		{"trim_trailing", "https://domain.com/api/", "https://domain.com/api", false},
		{"missing_scheme", "localhost:9999/api", "", true},
		{"empty", "   ", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeTargetURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestNormalizeExportOptions(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		opts, err := normalizeExportOptions(ExportRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !opts.IncludeMetadata || !opts.IncludeConstants || !opts.IncludeCatalogs || !opts.IncludeNomenclature {
			t.Fatalf("expected all includes enabled by default, got %+v", opts)
		}
		if opts.BatchSize != defaultExportBatchSize {
			t.Fatalf("expected default batch size %d, got %d", defaultExportBatchSize, opts.BatchSize)
		}
	})

	t.Run("custom_include_and_catalogs", func(t *testing.T) {
		req := ExportRequest{
			Include:      []string{"metadata", "catalogs"},
			CatalogNames: []string{" Номенклатура ", "Номенклатура", "Контрагенты", ""},
			BatchSize:    5,
		}
		opts, err := normalizeExportOptions(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !opts.IncludeMetadata || !opts.IncludeCatalogs {
			t.Fatalf("expected metadata & catalogs enabled")
		}
		if opts.IncludeConstants || opts.IncludeNomenclature {
			t.Fatalf("constants/nomenclature should be disabled")
		}
		expectedNames := []string{"Номенклатура", "Контрагенты"}
		if !reflect.DeepEqual(opts.CatalogNames, expectedNames) {
			t.Fatalf("unexpected catalog names: %+v", opts.CatalogNames)
		}
		if opts.BatchSize != 5 {
			t.Fatalf("unexpected batch size %d", opts.BatchSize)
		}
	})

	t.Run("invalid_include", func(t *testing.T) {
		_, err := normalizeExportOptions(ExportRequest{Include: []string{"unknown"}})
		if err == nil {
			t.Fatalf("expected error for unknown include")
		}
	})

	t.Run("batch_size_limits", func(t *testing.T) {
		opts, err := normalizeExportOptions(ExportRequest{BatchSize: 10_000})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.BatchSize != maxExportBatchSize {
			t.Fatalf("expected clamp to %d, got %d", maxExportBatchSize, opts.BatchSize)
		}

		opts, err = normalizeExportOptions(ExportRequest{BatchSize: 0})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.BatchSize != defaultExportBatchSize {
			t.Fatalf("expected default batch size, got %d", opts.BatchSize)
		}
	})
}


