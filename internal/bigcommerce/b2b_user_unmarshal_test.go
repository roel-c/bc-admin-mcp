package bigcommerce_test

import (
	"encoding/json"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/stretchr/testify/require"
)

func TestB2BUserUnmarshalAcceptsBCCustomerID(t *testing.T) {
	var u bigcommerce.B2BUser
	err := json.Unmarshal([]byte(`{"id":1,"companyId":2,"email":"a@b.com","role":0,"bcCustomerId":99}`), &u)
	require.NoError(t, err)
	require.Equal(t, 99, u.BCCustomerID)
}

func TestB2BUserUnmarshalAcceptsBCIDAlias(t *testing.T) {
	var u bigcommerce.B2BUser
	err := json.Unmarshal([]byte(`{"id":1,"companyId":2,"email":"a@b.com","role":0,"bcId":"136"}`), &u)
	require.NoError(t, err)
	require.Equal(t, 136, u.BCCustomerID)
}

func TestB2BUserUnmarshalPrefersBCCustomerIDOverBCID(t *testing.T) {
	var u bigcommerce.B2BUser
	err := json.Unmarshal([]byte(`{"id":1,"companyId":2,"email":"a@b.com","role":0,"bcCustomerId":10,"bcId":20}`), &u)
	require.NoError(t, err)
	require.Equal(t, 10, u.BCCustomerID)
}
