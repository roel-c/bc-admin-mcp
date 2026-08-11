package catalog

import (
	"fmt"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// ResolveVariantOptionValues fills option_id and value id on each
// VariantOptionVal when callers supply option_display_name + label (the
// agent-friendly shape). Values that already have both IDs are left unchanged.
// options must be the product's current option list from ListProductOptions.
func ResolveVariantOptionValues(options []bigcommerce.ProductOption, vals []bigcommerce.VariantOptionVal) ([]bigcommerce.VariantOptionVal, error) {
	if len(vals) == 0 {
		return vals, nil
	}

	out := make([]bigcommerce.VariantOptionVal, len(vals))
	copy(out, vals)

	needsResolve := false
	for i := range out {
		if out[i].ID == 0 || out[i].OptionID == 0 {
			needsResolve = true
			break
		}
	}
	if !needsResolve {
		return out, nil
	}

	byName := indexOptionsByDisplayName(options)

	for i := range out {
		if out[i].ID != 0 && out[i].OptionID != 0 {
			continue
		}
		if err := resolveOneVariantOptionValue(byName, options, &out[i], i); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func indexOptionsByDisplayName(options []bigcommerce.ProductOption) map[string][]bigcommerce.ProductOption {
	byName := make(map[string][]bigcommerce.ProductOption, len(options))
	for _, opt := range options {
		key := strings.ToLower(strings.TrimSpace(opt.DisplayName))
		byName[key] = append(byName[key], opt)
	}
	return byName
}

func resolveOneVariantOptionValue(
	byName map[string][]bigcommerce.ProductOption,
	all []bigcommerce.ProductOption,
	val *bigcommerce.VariantOptionVal,
	index int,
) error {
	label := strings.TrimSpace(val.Label)
	if label == "" {
		return fmt.Errorf("option_values[%d]: label is required", index)
	}

	// Option known by ID: resolve value label (or verify) within that option.
	if val.OptionID != 0 {
		opt, ok := findOptionByID(all, val.OptionID)
		if !ok {
			return fmt.Errorf("option_values[%d]: option_id %d not found on product", index, val.OptionID)
		}
		if val.ID == 0 {
			vid, err := findOptionValueID(opt, label)
			if err != nil {
				return fmt.Errorf("option_values[%d]: %w", index, err)
			}
			val.ID = vid
		}
		if val.OptionDisplayName == "" {
			val.OptionDisplayName = opt.DisplayName
		}
		return nil
	}

	// Value ID known but option_id missing: locate owning option.
	if val.ID != 0 {
		opt, ok := findOptionByValueID(all, val.ID)
		if !ok {
			return fmt.Errorf("option_values[%d]: value id %d not found on product options", index, val.ID)
		}
		val.OptionID = opt.ID
		if val.OptionDisplayName == "" {
			val.OptionDisplayName = opt.DisplayName
		}
		return nil
	}

	name := strings.TrimSpace(val.OptionDisplayName)
	if name == "" {
		return fmt.Errorf("option_values[%d]: option_display_name is required when option_id/id are omitted", index)
	}
	matches := byName[strings.ToLower(name)]
	if len(matches) == 0 {
		return fmt.Errorf("option_values[%d]: no option named %q on product", index, name)
	}
	if len(matches) > 1 {
		return fmt.Errorf("option_values[%d]: multiple options named %q — pass option_id", index, name)
	}
	opt := matches[0]
	vid, err := findOptionValueID(opt, label)
	if err != nil {
		return fmt.Errorf("option_values[%d]: %w", index, err)
	}
	val.OptionID = opt.ID
	val.ID = vid
	val.OptionDisplayName = opt.DisplayName
	return nil
}

func findOptionByID(options []bigcommerce.ProductOption, id int) (bigcommerce.ProductOption, bool) {
	for _, opt := range options {
		if opt.ID == id {
			return opt, true
		}
	}
	return bigcommerce.ProductOption{}, false
}

func findOptionByValueID(options []bigcommerce.ProductOption, valueID int) (bigcommerce.ProductOption, bool) {
	for _, opt := range options {
		for _, ov := range opt.OptionValues {
			if ov.ID == valueID {
				return opt, true
			}
		}
	}
	return bigcommerce.ProductOption{}, false
}

func findOptionValueID(opt bigcommerce.ProductOption, label string) (int, error) {
	want := strings.ToLower(strings.TrimSpace(label))
	var matches []bigcommerce.ProductOptionValue
	for _, ov := range opt.OptionValues {
		if strings.ToLower(strings.TrimSpace(ov.Label)) == want {
			matches = append(matches, ov)
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("option %q has no value labeled %q", opt.DisplayName, label)
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("option %q has multiple values labeled %q — pass id", opt.DisplayName, label)
	}
	return matches[0].ID, nil
}
