package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type customersDataEnvelope struct {
	Data []Customer `json:"data"`
}

type customerAddressesDataEnvelope struct {
	Data []CustomerAddress `json:"data"`
}

// SearchCustomers lists customers via GET /v3/customers with arbitrary query
// parameters (filters, include, sort, page, limit, cursors). Uses the
// client's offset pagination until MaxTotalRecords.
func (c *Client) SearchCustomers(ctx context.Context, params map[string]string) ([]Customer, error) {
	path := "customers"
	if len(params) > 0 {
		vals := url.Values{}
		for k, v := range params {
			if v != "" {
				vals.Set(k, v)
			}
		}
		if enc := vals.Encode(); enc != "" {
			path += "?" + enc
		}
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search customers: %w", err)
	}
	out := make([]Customer, 0, len(raw))
	for _, r := range raw {
		var cust Customer
		if err := json.Unmarshal(r, &cust); err != nil {
			return nil, fmt.Errorf("unmarshal customer: %w", err)
		}
		out = append(out, cust)
	}
	return out, nil
}

const maxCustomerIDInQuery = 40

// GetCustomersByIDs fetches customers by numeric IDs using GET /v3/customers?id:in=…
// (BigCommerce has no GET-by-single-id route). IDs are chunked to keep query strings small.
func (c *Client) GetCustomersByIDs(ctx context.Context, ids []int) ([]Customer, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("no customer ids provided")
	}
	var all []Customer
	for i := 0; i < len(ids); i += maxCustomerIDInQuery {
		end := i + maxCustomerIDInQuery
		if end > len(ids) {
			end = len(ids)
		}
		sub := ids[i:end]
		params := map[string]string{"id:in": joinInts(sub)}
		part, err := c.SearchCustomers(ctx, params)
		if err != nil {
			return nil, err
		}
		all = append(all, part...)
	}
	return all, nil
}

// CreateCustomers creates up to 10 customers per call via POST /v3/customers.
func (c *Client) CreateCustomers(ctx context.Context, payload []CustomerCreate) ([]Customer, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty create payload")
	}
	body, err := c.Post(ctx, "customers", payload)
	if err != nil {
		return nil, fmt.Errorf("create customers: %w", err)
	}
	var env customersDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create customers response: %w", err)
	}
	return env.Data, nil
}

// UpdateCustomers updates up to 10 customers per call via PUT /v3/customers.
func (c *Client) UpdateCustomers(ctx context.Context, payload []CustomerUpdate) ([]Customer, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty update payload")
	}
	body, err := c.Put(ctx, "customers", payload)
	if err != nil {
		return nil, fmt.Errorf("update customers: %w", err)
	}
	var env customersDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update customers response: %w", err)
	}
	return env.Data, nil
}

// DeleteCustomers deletes customers via DELETE /v3/customers?id:in=…
func (c *Client) DeleteCustomers(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no customer ids provided")
	}
	q := url.Values{}
	q.Set("id:in", joinInts(ids))
	_, err := c.Delete(ctx, "customers?"+q.Encode())
	if err != nil {
		return fmt.Errorf("delete customers: %w", err)
	}
	return nil
}

// SearchCustomerAddresses lists addresses via GET /v3/customers/addresses.
func (c *Client) SearchCustomerAddresses(ctx context.Context, params map[string]string) ([]CustomerAddress, error) {
	path := "customers/addresses"
	if len(params) > 0 {
		vals := url.Values{}
		for k, v := range params {
			if v != "" {
				vals.Set(k, v)
			}
		}
		if enc := vals.Encode(); enc != "" {
			path += "?" + enc
		}
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search customer addresses: %w", err)
	}
	out := make([]CustomerAddress, 0, len(raw))
	for _, r := range raw {
		var addr CustomerAddress
		if err := json.Unmarshal(r, &addr); err != nil {
			return nil, fmt.Errorf("unmarshal customer address: %w", err)
		}
		out = append(out, addr)
	}
	return out, nil
}

// CreateCustomerAddresses creates one or more addresses via POST /v3/customers/addresses.
func (c *Client) CreateCustomerAddresses(ctx context.Context, payload []CustomerAddressCreate) ([]CustomerAddress, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty address create payload")
	}
	body, err := c.Post(ctx, "customers/addresses", payload)
	if err != nil {
		return nil, fmt.Errorf("create customer addresses: %w", err)
	}
	var env customerAddressesDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse create addresses response: %w", err)
	}
	return env.Data, nil
}

// UpdateCustomerAddresses updates addresses via PUT /v3/customers/addresses.
func (c *Client) UpdateCustomerAddresses(ctx context.Context, payload []CustomerAddressUpdate) ([]CustomerAddress, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty address update payload")
	}
	body, err := c.Put(ctx, "customers/addresses", payload)
	if err != nil {
		return nil, fmt.Errorf("update customer addresses: %w", err)
	}
	var env customerAddressesDataEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse update addresses response: %w", err)
	}
	return env.Data, nil
}

// DeleteCustomerAddresses deletes addresses via DELETE /v3/customers/addresses?id:in=…
func (c *Client) DeleteCustomerAddresses(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("no address ids provided")
	}
	q := url.Values{}
	q.Set("id:in", joinInts(ids))
	_, err := c.Delete(ctx, "customers/addresses?"+q.Encode())
	if err != nil {
		return fmt.Errorf("delete customer addresses: %w", err)
	}
	return nil
}
