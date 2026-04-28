package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// RegisterImageTools registers the image sub-resource tools.
func (p *Products) RegisterImageTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/images/list",
		Tier:        middleware.TierR0,
		Summary:     "List all images for a product",
		Description: "Returns images with URLs, sort order, thumbnail flag, and dimensions.",
		Tool: mcp.NewTool("catalog_products_images_list",
			mcp.WithDescription("List all images for a product by product_id."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
		),
		Handler: p.handleImageList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/images/add",
		Tier:    middleware.TierR1,
		Summary: "Add an image to a product by URL (JPEG, PNG, GIF, WebP)",
		Description: "Creates a product image from a URL. BigCommerce downloads and hosts the image. " +
			"Supports JPEG/JPG, PNG, GIF, and WebP formats. Max 8 MB per image.",
		Tool: mcp.NewTool("catalog_products_images_add",
			mcp.WithDescription(
				"Add an image to a product by providing a URL. "+
					"Supported formats: JPEG/JPG, PNG, GIF, WebP. Max 8 MB. "+
					"Preview shows the proposed image; pass confirmed=true to create.",
			),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithString("image_url", mcp.Description("Public URL of the image to add"), mcp.Required()),
			mcp.WithBoolean("is_thumbnail", mcp.Description("Set as the product thumbnail image")),
			mcp.WithNumber("sort_order", mcp.Description("Display sort order")),
			mcp.WithString("description", mcp.Description("Image alt text / description")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview to add the image")),
		),
		Handler: p.handleImageAdd,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/images/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete a product image",
		Description: "Removes an image from a product. This action cannot be undone. " +
			"Preview shows image details; pass confirmed=true to delete.",
		Tool: mcp.NewTool("catalog_products_images_delete",
			mcp.WithDescription("Delete a product image by product_id and image_id. Preview first; pass confirmed=true to execute."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("image_id", mcp.Description("Image ID to delete"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview to delete")),
		),
		Handler: p.handleImageDelete,
	})
}

func (p *Products) handleImageList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, err := requiredPositiveInt(request.GetArguments(), "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	images, err := p.bc.ListProductImages(ctx, productID)
	if err != nil {
		return toolError("failed to list images: %v", err), nil
	}

	return toolJSON(map[string]any{
		"product_id":   productID,
		"total_images": len(images),
		"images":       images,
	})
}

var validImageExtensions = []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}

func validateImageURL(url string) error {
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return fmt.Errorf("image_url must start with http:// or https://")
	}
	// Strip query params for extension check
	path := lower
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	for _, ext := range validImageExtensions {
		if strings.HasSuffix(path, ext) {
			return nil
		}
	}
	return fmt.Errorf("image_url must end in .jpg, .jpeg, .png, .gif, or .webp")
}

func (p *Products) handleImageAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	imageURL, ok := args["image_url"].(string)
	if !ok || imageURL == "" {
		return toolError("image_url is required"), nil
	}
	if err := validateImageURL(imageURL); err != nil {
		return toolError("%s", err.Error()), nil
	}

	payload := bigcommerce.ProductImageCreate{ImageURL: imageURL}
	if v, ok := args["is_thumbnail"].(bool); ok {
		payload.IsThumbnail = v
	}
	if v, ok := args["sort_order"].(float64); ok {
		payload.SortOrder = int(v)
	}
	if v, ok := args["description"].(string); ok {
		payload.Description = v
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"image_url":  imageURL,
			"payload":    payload,
			"message":    "Image will be added to the product. Pass confirmed=true to execute.",
		})
	}

	img, err := p.bc.CreateProductImage(ctx, productID, payload)
	if err != nil {
		return toolError("failed to add image: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"image":      img,
	})
}

func (p *Products) handleImageDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	imageID, err := requiredPositiveInt(args, "image_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		images, lErr := p.bc.ListProductImages(ctx, productID)
		if lErr != nil {
			return toolError("failed to list images for preview: %v", lErr), nil
		}
		var target *bigcommerce.ProductImage
		for i := range images {
			if images[i].ID == imageID {
				target = &images[i]
				break
			}
		}
		if target == nil {
			return toolError("image %d not found on product %d", imageID, productID), nil
		}
		return toolJSON(map[string]any{
			"status":       "pending_confirmation",
			"product_id":   productID,
			"image_id":     imageID,
			"image_file":   target.ImageFile,
			"is_thumbnail": target.IsThumbnail,
			"message":      "This image will be permanently deleted. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.DeleteProductImage(ctx, productID, imageID); err != nil {
		return toolError("failed to delete image: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"image_id":   imageID,
		"message":    "Image deleted successfully.",
	})
}

func requiredPositiveInt(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	f, fOk := v.(float64)
	if !fOk || f <= 0 || f != float64(int(f)) {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return int(f), nil
}
