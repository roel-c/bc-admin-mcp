package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// BrandSearchFilters maps tool parameters to BigCommerce brand list query keys.
var BrandSearchFilters = []SearchFilter{
	{ToolKey: "name", BCKey: "name", Kind: "string"},
	{ToolKey: "name_like", BCKey: "name:like", Kind: "string"},
	{ToolKey: "keyword", BCKey: "keyword", Kind: "string"},
	{ToolKey: "page_title", BCKey: "page_title", Kind: "string"},
	{ToolKey: "page_title_like", BCKey: "page_title:like", Kind: "string"},
	{ToolKey: "id", BCKey: "id", Kind: "number"},
	{ToolKey: "sort", BCKey: "sort", Kind: "string"},
	{ToolKey: "sort_direction", BCKey: "direction", Kind: "string"},
}

var brandNonFilterKeys = map[string]bool{
	"sort": true, "sort_direction": true,
}

var validBrandSortFields = map[string]bool{
	"id": true, "name": true, "date_modified": true,
}

// Brands provides MCP tool handlers for catalog brand operations.
type Brands struct {
	bc BigCommerceAPI
}

// NewBrands constructs brand tool handlers.
func NewBrands(bc BigCommerceAPI) *Brands {
	return &Brands{bc: bc}
}

// RegisterTools registers brand list/get/create/update tools.
func (b *Brands) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/list",
		Tier:    middleware.TierR0,
		Summary: "List or search brands by name, keyword, page title, or ID",
		Description: "Fetches brands from GET /v3/catalog/brands. Provide at least one filter " +
			"or set list_all=true to return every brand.",
		Tool: mcp.NewTool("catalog_brands_list",
			mcp.WithDescription(
				"List or search brands. Pass list_all=true for every brand, "+
					"or filters such as name, name_like, keyword, page_title, or id.",
			),
			mcp.WithBoolean("list_all",
				mcp.Description("Set to true to return all brands in the store."),
			),
			mcp.WithString("name",
				mcp.Description("Exact brand name match."),
			),
			mcp.WithString("name_like",
				mcp.Description("Partial name match (SQL LIKE)."),
			),
			mcp.WithString("keyword",
				mcp.Description("Keyword search across brand fields."),
			),
			mcp.WithString("page_title",
				mcp.Description("Exact SEO page title match."),
			),
			mcp.WithString("page_title_like",
				mcp.Description("Partial page title match (SQL LIKE)."),
			),
			mcp.WithNumber("id",
				mcp.Description("Filter by brand ID."),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field: id, name, or date_modified (default: id)."),
			),
			mcp.WithString("sort_direction",
				mcp.Description("Sort direction: asc or desc."),
			),
		),
		Handler: b.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/brands/get",
		Tier:        middleware.TierR0,
		Summary:     "Get a single brand by ID",
		Description: "Fetches one brand via GET /v3/catalog/brands/{id} including SEO and image URL.",
		Tool: mcp.NewTool("catalog_brands_get",
			mcp.WithDescription("Get brand details by numeric brand_id."),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID"), mcp.Required()),
		),
		Handler: b.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new catalog brand",
		Description: "POST /v3/catalog/brands. Requires name; optional SEO fields, image URL, layout file, " +
			"and custom URL path. Returns a preview first; pass confirmed=true to create.",
		Tool: mcp.NewTool("catalog_brands_create",
			mcp.WithDescription(
				"Create a brand. Provide name (required). Preview first; pass confirmed=true to execute.",
			),
			mcp.WithString("name", mcp.Description("Brand name (required)."), mcp.Required()),
			mcp.WithString("page_title", mcp.Description("SEO page title.")),
			mcp.WithString("meta_description", mcp.Description("SEO meta description.")),
			mcp.WithString("search_keywords", mcp.Description("Comma-separated search keywords.")),
			mcp.WithString("image_url", mcp.Description("Brand logo or image URL.")),
			mcp.WithString("layout_file", mcp.Description("Stencil layout file name.")),
			mcp.WithString("custom_url",
				mcp.Description("Storefront URL path for the brand (maps to custom_url)."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: b.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/update",
		Tier:    middleware.TierR1,
		Summary: "Update an existing brand",
		Description: "PUT /v3/catalog/brands/{id}. Only supplied fields are changed. " +
			"Preview first; pass confirmed=true to apply.",
		Tool: mcp.NewTool("catalog_brands_update",
			mcp.WithDescription(
				"Update a brand by brand_id. Include any fields to change. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID to update."), mcp.Required()),
			mcp.WithString("name", mcp.Description("New brand name.")),
			mcp.WithString("page_title", mcp.Description("New SEO page title.")),
			mcp.WithString("meta_description", mcp.Description("New SEO meta description.")),
			mcp.WithString("search_keywords", mcp.Description("New search keywords.")),
			mcp.WithString("image_url", mcp.Description("New image URL.")),
			mcp.WithString("layout_file", mcp.Description("New layout file name.")),
			mcp.WithString("custom_url",
				mcp.Description("New storefront URL path (maps to custom_url)."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: b.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a brand by ID",
		Description: "DELETE /v3/catalog/brands/{id}. Destructive and irreversible. Products keep existing " +
			"but their brand association is cleared. Preview shows the brand; pass confirmed=true to delete.",
		Tool: mcp.NewTool("catalog_brands_delete",
			mcp.WithDescription("Delete a brand by brand_id. Preview first; pass confirmed=true to permanently delete."),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID to delete."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to permanently delete after reviewing the preview.")),
		),
		Handler: b.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/image/set",
		Tier:    middleware.TierR1,
		Summary: "Set or replace a brand's image by URL",
		Description: "Sets the brand image via PUT /v3/catalog/brands/{id} with image_url (BigCommerce fetches " +
			"the publicly accessible URL). Direct file uploads (multipart) are not supported here — host the " +
			"image and pass its URL. Preview first; pass confirmed=true to apply.",
		Tool: mcp.NewTool("catalog_brands_image_set",
			mcp.WithDescription("Set/replace a brand's image using a publicly accessible image URL. Preview first; confirmed=true to apply."),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID."), mcp.Required()),
			mcp.WithString("image_url", mcp.Description("Publicly accessible image URL (jpg/png/gif/webp)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to apply after reviewing the preview.")),
		),
		Handler: b.handleImageSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/brands/image/delete",
		Tier:        middleware.TierR2,
		Summary:     "Remove a brand's image",
		Description: "DELETE /v3/catalog/brands/{id}/image. Removes the brand's current image. Preview first; pass confirmed=true.",
		Tool: mcp.NewTool("catalog_brands_image_delete",
			mcp.WithDescription("Remove a brand's image by brand_id. Preview first; pass confirmed=true."),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to remove the image after reviewing the preview.")),
		),
		Handler: b.handleImageDelete,
	})

	b.registerBrandMetafieldTools(reg)
}

func (b *Brands) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	brandID, err := requiredPositiveInt(args, "brand_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	current, err := b.bc.GetBrand(ctx, brandID)
	if err != nil {
		return toolError("failed to fetch brand %d: %v", brandID, err), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":   "pending_confirmation",
			"action":   "delete_brand",
			"brand":    map[string]any{"id": current.ID, "name": current.Name},
			"message":  fmt.Sprintf("WARNING: brand %d (%q) will be PERMANENTLY DELETED. Products keep existing but lose this brand association. Pass confirmed=true to execute.", current.ID, current.Name),
		})
	}

	if err := b.bc.DeleteBrand(ctx, brandID); err != nil {
		return toolError("failed to delete brand %d: %v", brandID, err), nil
	}
	return toolJSON(map[string]any{"status": "deleted", "brand_id": brandID})
}

func (b *Brands) handleImageSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	brandID, err := requiredPositiveInt(args, "brand_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	imageURL, ok := args["image_url"].(string)
	if !ok || strings.TrimSpace(imageURL) == "" {
		return toolError("image_url is required and must be a non-empty string"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":    "pending_confirmation",
			"action":    "set_brand_image",
			"brand_id":  brandID,
			"image_url": imageURL,
			"message":   "The brand image will be set to this URL (BigCommerce fetches it server-side). Pass confirmed=true to apply.",
		})
	}

	updated, err := b.bc.UpdateBrand(ctx, brandID, bigcommerce.BrandUpdate{ImageURL: &imageURL})
	if err != nil {
		return toolError("failed to set brand %d image: %v", brandID, err), nil
	}
	return toolJSON(map[string]any{"status": "updated", "brand": updated})
}

func (b *Brands) handleImageDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	brandID, err := requiredPositiveInt(args, "brand_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":   "pending_confirmation",
			"action":   "delete_brand_image",
			"brand_id": brandID,
			"message":  fmt.Sprintf("The image on brand %d will be removed. Pass confirmed=true to execute.", brandID),
		})
	}

	if err := b.bc.DeleteBrandImage(ctx, brandID); err != nil {
		return toolError("failed to delete brand %d image: %v", brandID, err), nil
	}
	return toolJSON(map[string]any{"status": "deleted", "brand_id": brandID, "message": "Brand image removed."})
}

func (b *Brands) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	listAll := ReadListAllBoolean(args, "list_all")

	params, err := ExtractFilters(args, BrandSearchFilters)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	hasDataFilter := HasDataFilterBCParams(params, BrandSearchFilters, brandNonFilterKeys)

	if !hasDataFilter && !listAll {
		return toolError(
			"provide at least one filter (name, name_like, keyword, page_title, page_title_like, id) " +
				"or set list_all=true to return every brand.",
		), nil
	}

	if err := ErrInvalidBCSort(params, validBrandSortFields, "valid options: id, name, date_modified"); err != nil {
		return toolError("%s", err.Error()), nil
	}

	brands, err := b.bc.SearchBrands(ctx, params)
	if err != nil {
		return toolError("failed to search brands: %v", err), nil
	}

	type brandSummary struct {
		ID              int    `json:"id"`
		Name            string `json:"name"`
		PageTitle       string `json:"page_title,omitempty"`
		MetaDescription string `json:"meta_description,omitempty"`
		ImageURL        string `json:"image_url,omitempty"`
	}

	summaries := make([]brandSummary, len(brands))
	for i, br := range brands {
		summaries[i] = brandSummary{
			ID:              br.ID,
			Name:            br.Name,
			PageTitle:       br.PageTitle,
			MetaDescription: br.MetaDescription,
			ImageURL:        br.ImageURL,
		}
	}

	return toolJSON(map[string]any{
		"total":  len(brands),
		"brands": summaries,
	})
}

func (b *Brands) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	raw, ok := args["brand_id"]
	if !ok {
		return toolError("brand_id is required"), nil
	}
	f, ok := raw.(float64)
	if !ok {
		return toolError("brand_id must be a number"), nil
	}
	id := int(f)

	brand, err := b.bc.GetBrand(ctx, id)
	if err != nil {
		return toolError("failed to get brand %d: %v", id, err), nil
	}
	return toolJSON(brand)
}

func (b *Brands) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseBrandCreateParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if params.Confirmed {
		created, err := b.bc.CreateBrand(ctx, params.Payload)
		if err != nil {
			return toolError("failed to create brand: %v", err), nil
		}
		return toolJSON(map[string]any{
			"status":  "created",
			"message": fmt.Sprintf("Brand %q created successfully with ID %d.", created.Name, created.ID),
			"brand": map[string]any{
				"id":   created.ID,
				"name": created.Name,
			},
		})
	}

	return b.previewBrandCreate(params)
}

func (b *Brands) previewBrandCreate(params *BrandCreateParams) (*mcp.CallToolResult, error) {
	m := map[string]any{
		"name": params.Payload.Name,
	}
	if params.Payload.PageTitle != "" {
		m["page_title"] = params.Payload.PageTitle
	}
	if params.Payload.MetaDescription != "" {
		m["meta_description"] = params.Payload.MetaDescription
	}
	if params.Payload.SearchKeywords != "" {
		m["search_keywords"] = params.Payload.SearchKeywords
	}
	if params.Payload.ImageURL != "" {
		m["image_url"] = params.Payload.ImageURL
	}
	if params.Payload.LayoutFile != "" {
		m["layout_file"] = params.Payload.LayoutFile
	}
	if params.Payload.CustomURL != nil {
		m["custom_url"] = params.Payload.CustomURL.GetPath()
	}

	return toolJSON(map[string]any{
		"status":  "preview",
		"message": "Review the brand below. Pass confirmed=true with the same parameters to create it.",
		"brand":   m,
	})
}

func (b *Brands) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseBrandUpdateParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !params.hasFields() {
		return toolError("provide at least one field to update (name, page_title, meta_description, search_keywords, image_url, layout_file, custom_url)"), nil
	}

	if params.Confirmed {
		updated, err := b.bc.UpdateBrand(ctx, params.BrandID, params.Update)
		if err != nil {
			return toolError("failed to update brand %d: %v", params.BrandID, err), nil
		}
		return toolJSON(map[string]any{
			"status":  "updated",
			"message": fmt.Sprintf("Brand %d updated successfully.", params.BrandID),
			"brand": map[string]any{
				"id":   updated.ID,
				"name": updated.Name,
			},
		})
	}

	return b.previewBrandUpdate(params)
}

func (b *Brands) previewBrandUpdate(params *BrandUpdateParams) (*mcp.CallToolResult, error) {
	changes := map[string]any{"brand_id": params.BrandID}
	if params.Update.Name != nil {
		changes["name"] = *params.Update.Name
	}
	if params.Update.PageTitle != nil {
		changes["page_title"] = *params.Update.PageTitle
	}
	if params.Update.MetaDescription != nil {
		changes["meta_description"] = *params.Update.MetaDescription
	}
	if params.Update.SearchKeywords != nil {
		changes["search_keywords"] = *params.Update.SearchKeywords
	}
	if params.Update.ImageURL != nil {
		changes["image_url"] = *params.Update.ImageURL
	}
	if params.Update.LayoutFile != nil {
		changes["layout_file"] = *params.Update.LayoutFile
	}
	if params.Update.CustomURL != nil {
		changes["custom_url"] = params.Update.CustomURL.GetPath()
	}

	return toolJSON(map[string]any{
		"status":  "preview",
		"message": fmt.Sprintf("Review changes for brand %d. Pass confirmed=true to apply.", params.BrandID),
		"changes": changes,
	})
}

// BrandCreateParams holds parsed create-brand arguments.
type BrandCreateParams struct {
	Payload   bigcommerce.BrandCreate
	Confirmed bool
}

// ParseBrandCreateParams parses catalog_brands_create arguments.
func ParseBrandCreateParams(args map[string]any) (*BrandCreateParams, error) {
	p := &BrandCreateParams{}

	nameRaw, ok := args["name"]
	if !ok {
		return nil, fmt.Errorf("name is required")
	}
	name, ok := nameRaw.(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name must be a non-empty string")
	}
	p.Payload.Name = name

	if v, ok := args["page_title"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("page_title must be a string")
		}
		p.Payload.PageTitle = s
	}
	if v, ok := args["meta_description"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("meta_description must be a string")
		}
		p.Payload.MetaDescription = s
	}
	if v, ok := args["search_keywords"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("search_keywords must be a string")
		}
		p.Payload.SearchKeywords = s
	}
	if v, ok := args["image_url"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("image_url must be a string")
		}
		p.Payload.ImageURL = s
	}
	if v, ok := args["layout_file"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("layout_file must be a string")
		}
		p.Payload.LayoutFile = s
	}
	if v, ok := args["custom_url"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("custom_url must be a string")
		}
		if s != "" {
			p.Payload.CustomURL = &bigcommerce.CustomURL{URL: s}
		}
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}

// BrandUpdateParams holds parsed update-brand arguments.
type BrandUpdateParams struct {
	BrandID   int
	Update    bigcommerce.BrandUpdate
	Confirmed bool
}

func (p *BrandUpdateParams) hasFields() bool {
	u := p.Update
	return u.Name != nil || u.PageTitle != nil || u.MetaDescription != nil ||
		u.SearchKeywords != nil || u.ImageURL != nil || u.LayoutFile != nil || u.CustomURL != nil
}

// ParseBrandUpdateParams parses catalog_brands_update arguments.
func ParseBrandUpdateParams(args map[string]any) (*BrandUpdateParams, error) {
	p := &BrandUpdateParams{}

	idRaw, ok := args["brand_id"]
	if !ok {
		return nil, fmt.Errorf("brand_id is required")
	}
	f, ok := idRaw.(float64)
	if !ok {
		return nil, fmt.Errorf("brand_id must be a number")
	}
	p.BrandID = int(f)
	if p.BrandID <= 0 {
		return nil, fmt.Errorf("brand_id must be positive")
	}

	if v, ok := args["name"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("name must be a string")
		}
		p.Update.Name = &s
	}
	if v, ok := args["page_title"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("page_title must be a string")
		}
		p.Update.PageTitle = &s
	}
	if v, ok := args["meta_description"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("meta_description must be a string")
		}
		p.Update.MetaDescription = &s
	}
	if v, ok := args["search_keywords"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("search_keywords must be a string")
		}
		p.Update.SearchKeywords = &s
	}
	if v, ok := args["image_url"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("image_url must be a string")
		}
		p.Update.ImageURL = &s
	}
	if v, ok := args["layout_file"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("layout_file must be a string")
		}
		p.Update.LayoutFile = &s
	}
	if v, ok := args["custom_url"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("custom_url must be a string")
		}
		p.Update.CustomURL = &bigcommerce.CustomURL{URL: s}
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}
