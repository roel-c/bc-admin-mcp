# B2BE Page Detection — Research Findings

## Overview

This document captures the findings from hands-on investigation into how BigCommerce B2B Edition (B2BE) deploys its buyer portal on a Stencil storefront, and how external scripts can reliably detect which B2BE page is active and inject custom behavior.

---

## Architecture Summary

B2BE is a **React 18 SPA** (built with Vite, MUI 5, React Router 6) loaded via two Script Manager scripts that BC auto-installs when B2BE is enabled on a channel.

### Deployment scripts (auto-installed by BC)

| Script name | Location | Pages |
|---|---|---|
| `B2BEdition Header Script` | Head | All pages |
| `B2BEdition Footer Script` | Footer | All pages |

> These are managed by BC and should **not** be manually recreated. To restore them: B2B Edition App → Settings → Buyer Portal → set **Buyer portal type = Default**.

**Header script** — hides the page body on `/account.php` for logged-in B2B users while the portal mounts:
```html
<script>
  {{#if customer.id}}
  {{#contains page_type "account"}}
  var b2bHideBodyStyle = document.createElement('style');
  b2bHideBodyStyle.id = 'b2b-account-page-hide-body';
  b2bHideBodyStyle.innerHTML = 'body { display: none !important }';
  document.head.appendChild(b2bHideBodyStyle);
  {{/contains}}
  {{/if}}
</script>
```

**Footer script** — initializes `window.B3` config and loads the BC-hosted React bundle:
```html
<script>
  window.b3CheckoutConfig = { routes: { dashboard: '/account.php?action=order_status' } }
  window.B3 = {
    setting: {
      store_hash: '{{settings.store_hash}}',
      channel_id: {{settings.channel_id}},
      platform: 'bigcommerce'
    },
    'dom.checkoutRegisterParentElement': '#checkout-app',
    'dom.registerElement': '[href^="/login.php"], ...',
    'dom.openB3Checkout': 'checkout-customer-continue',
    'dom.navUserLoginElement': '.navUser-item.navUser-item--account',
    before_login_goto_page: '/account.php?action=order_status',
    checkout_super_clear_session: 'true'
  }
</script>
<script type="module" crossorigin="" src="[BC-CDN-URL]/index.[hash].js"></script>
<script nomodule="" crossorigin="" src="[BC-CDN-URL]/polyfills-legacy.[hash].js"></script>
<script nomodule="" crossorigin="" src="[BC-CDN-URL]/index-legacy.[hash].js"></script>
```

---

## Rendering Architecture

### Confirmed architecture (both default and custom deployments)

**The portal DOM content is always in `<iframe class="active-frame">`** — this holds true for both default (BC-hosted) and custom (self-hosted) B2BE scripts. The iframe is same-origin and fully accessible via `frame.contentDocument`.

```
Outer page (Stencil)
│
├── window.B3              ← B2BE config (synchronous, all pages)
├── window.b2b             ← B2BE SDK (async, ~1-2s, portal pages)
├── window.location.hash   ← Current B2BE route (default scripts)
│     e.g. #/orders, #/quoteDraft
│
└── <iframe class="active-frame">   ← Portal DOM lives here
      └── contentDocument
            └── [React-rendered B2BE UI]
                  └── h3 "My orders", quote tables, etc.
```

**The critical distinction between deployment types is WHERE the route hash is stored:**

| Deployment | Route hash location | DOM location |
|---|---|---|
| **Default (BC-hosted)** | `window.location.hash` on outer page | `iframe.active-frame.contentDocument` |
| **Custom (self-hosted WebDAV)** | `iDoc.location.hash` inside iframe | `iframe.active-frame.contentDocument` |

> **Confirmed by injection test:** Running `injectBanner(getPortalDoc())` with default BC scripts returned `[B2BInjector] Injected into: iframe doc` — the h3 heading "My orders" was found and the element was successfully inserted via `iframe.active-frame.contentDocument`, not the outer `document`.

> **Note on custom deployments:** A custom B2BE build was used in initial testing, hosted on WebDAV at `/content/b2b-portal/`. When those files were unavailable (404), B2BE failed silently and left the BC account page broken (body hidden, portal never mounting). Reverting via B2B Edition App → Settings → Buyer Portal → **Default** restores the BC-managed scripts.

---

## Detection Signals

### Signal 1: `window.B3` — B2BE is configured on this channel

```javascript
const isB2BEChannel = !!(window.B3 && window.B3.setting);
```

- Available: **synchronously**, as soon as the footer script runs
- Present on: **all pages** of the B2BE channel (homepage, PDP, cart, portal pages)
- Does NOT indicate which portal page is active — just that B2BE is configured

### Signal 2: `window.location.hash` — current B2BE portal page (default scripts)

```javascript
const hash = window.location.hash; // e.g. '#/quoteDraft'
const isPortalPage = hash.startsWith('#/') && hash.length > 2;
```

- `hashchange` events fire on SPA navigation **within** the portal
- Initial navigation uses `history.replaceState` — **does NOT fire `hashchange`**
- Requires polling or a short init timer (~500ms) to catch the initial `replaceState`

### Signal 3: `window.b2b.utils` — B2BE SDK is ready

```javascript
const sdkReady = !!(window.b2b && window.b2b.utils);
```

- Available: **asynchronously**, ~1–2 seconds after page load
- Full SDK surface confirmed via console inspection on the My Orders portal page

**Confirmed `window.b2b` top-level keys:**

| Key | Type | Description |
|---|---|---|
| `utils` | object | Primary SDK namespace (see below) |
| `callbacks` | `Map(0)` | Event callback registry — subscribe to B2BE events |
| `initializationEnvironment` | function | B2BE initialization hook |
| `__get_asset_location` | function | Internal asset URL resolver |

**Confirmed `window.b2b.utils` namespaces:**

```javascript
// Navigation
b2b.utils.getRoutes()          // returns available B2BE routes
b2b.utils.openPage(route)      // programmatically navigate to a B2BE page
b2b.utils.setConfig(config)    // update B2BE runtime configuration

// Quote utilities
b2b.utils.quote.addProductFromPage()
b2b.utils.quote.addProductsFromCartId()
b2b.utils.quote.getProducts()
b2b.utils.quote.getQuoteConfig()
b2b.utils.quote.getCurrent()
b2b.utils.quote.getButtonInfo()
b2b.utils.quote.getRecentOrders()
b2b.utils.quote.getFilteredQuoteInfo()

// User utilities
b2b.utils.user.getProfile()
b2b.utils.user.getBaseMasqueradeInfo()
b2b.utils.user.getB2BToken()          // ← documented in BC dev center
b2b.utils.user.setMasquerade()
b2b.utils.user.getExpiredSessionRedirectUrl()
b2b.utils.user.loginWithB2BUserInfo()
b2b.utils.user.logout()
b2b.utils.user.logoutWithRedirectHash()

// Shopping list utilities
b2b.utils.shoppingList.addProductFromPage()
b2b.utils.shoppingList.getProducts()
b2b.utils.shoppingList.getCreatedShoppingList()
b2b.utils.shoppingList.updateList()

// Cart utilities
b2b.utils.cart.getCartInfo()
b2b.utils.cart.getEntityById()
b2b.utils.cart.getEntityByInfo()
```

> **`b2b.callbacks`** is a `Map` — subscribing entries to it may be the correct way to listen for B2BE page transitions and events without polling. This should be investigated further.

### Signal 4: `iframe.active-frame` — portal rendering container

```javascript
const frame = document.querySelector('iframe.active-frame');
```

- Present on B2BE portal pages (and as infrastructure on some storefront pages)
- Same-origin — `frame.contentDocument` is accessible
- With **default scripts**: hash is in outer page, not inside the iframe
- With **custom scripts**: hash is inside `iDoc.location.hash`

---

## Known B2BE Hash Routes

Matched using priority-ordered substring search (more specific routes first):

| Hash | Page label |
|---|---|
| `#/quoteDraft` | Quote Draft |
| `#/quoteDetail` | Quote Detail |
| `#/company-orders` | Company Orders |
| `#/shoppingLists` | Shopping Lists |
| `#/quickOrder` | Quick Order |
| `#/accountSettings` | Account Settings |
| `#/dashboard` | Dashboard |
| `#/invoices` | Invoices |
| `#/invoice` | Invoice |
| `#/orders` | My Orders |
| `#/quotes` | Quotes |
| `#/addresses` | Addresses |
| `#/users` | User Management |

> **Important:** Route matching must use a priority-ordered array (not an object), checked from most-specific to least-specific. `#/company-orders` contains `orders` as a substring — if `orders` is checked first, it produces a false match ("My Orders" instead of "Company Orders").

---

## Reliable Detection Pattern

```javascript
(function() {
    // Signal 1: B2BE channel
    var isB2BEChannel = !!(window.B3 && window.B3.setting);
    if (!isB2BEChannel) return; // not a B2BE channel page

    // Signal 2 + 3: Portal page and SDK
    var ROUTES = [
        ['quoteDraft',     'Quote Draft'],
        ['quoteDetail',    'Quote Detail'],
        ['company-orders', 'Company Orders'],
        ['shoppingLists',  'Shopping Lists'],
        ['quickOrder',     'Quick Order'],
        ['accountSettings','Account Settings'],
        ['dashboard',      'Dashboard'],
        ['invoices',       'Invoices'],
        ['invoice',        'Invoice'],
        ['orders',         'My Orders'],
        ['quotes',         'Quotes'],
        ['addresses',      'Addresses'],
        ['users',          'User Management'],
    ];

    function getPageLabel(hash) {
        for (var i = 0; i < ROUTES.length; i++) {
            if (hash.indexOf(ROUTES[i][0]) !== -1) return ROUTES[i][1];
        }
        return hash.replace('#/', '');
    }

    function isB2BERoute(hash) {
        return !!(hash && hash.indexOf('#/') === 0 && hash.length > 2);
    }

    function getCurrentPage() {
        var outerHash = window.location.hash || '';
        var host = window.location.hostname;

        // Check iframe first (custom script deployments)
        var iframes = document.querySelectorAll('iframe');
        for (var f = 0; f < iframes.length; f++) {
            var fr = iframes[f];
            var isActive = fr.className && fr.className.indexOf('active-frame') !== -1;
            try {
                var iDoc = fr.contentDocument || fr.contentWindow.document;
                var url = (iDoc.location && iDoc.location.href) || '';
                var iframeHash = (iDoc.location && iDoc.location.hash) || '';
                var isSameOrigin = url.indexOf(host) !== -1 && url !== 'about:blank';
                if ((isActive || isSameOrigin) && isB2BERoute(iframeHash)) {
                    return { hash: iframeHash, source: 'iframe', page: getPageLabel(iframeHash) };
                }
            } catch(e) {}
        }

        // Fall back to outer page hash (default BC-hosted deployment)
        if (isB2BERoute(outerHash)) {
            return { hash: outerHash, source: 'outer-page', page: getPageLabel(outerHash) };
        }

        return null; // B2BE channel but not on a portal page
    }

    // Initial page detection
    // history.replaceState (used by B2BE on first load) does NOT fire hashchange.
    // Use a short polling init to catch it, then rely on hashchange for subsequent navigation.
    var _initTimer = setInterval(function() {
        var page = getCurrentPage();
        if (page) {
            clearInterval(_initTimer);
            onPortalPage(page);
        }
    }, 500);
    setTimeout(function() { clearInterval(_initTimer); }, 15000);

    // Subsequent SPA navigations fire hashchange
    window.addEventListener('hashchange', function() {
        var page = getCurrentPage();
        if (page) onPortalPage(page);
        else onNonPortalPage();
    });

    function onPortalPage(page) {
        console.log('[B2BE] Portal page:', page.page, '| hash:', page.hash, '| source:', page.source);
        // → Add page-specific behavior here
    }

    function onNonPortalPage() {
        console.log('[B2BE] Not on portal page (storefront)');
    }
})();
```

---

## DOM Injection

### Confirmed injection pattern (validated on My Orders page)

The following injection was confirmed working: a green styled banner was successfully inserted after the "My orders" `h3` heading on the My Orders portal page using default BC-hosted B2BE scripts.

```javascript
function getPortalDoc() {
    var host = window.location.hostname;
    var iframes = document.querySelectorAll('iframe');
    for (var f = 0; f < iframes.length; f++) {
        var fr = iframes[f];
        var isActive = fr.className && fr.className.indexOf('active-frame') !== -1;
        try {
            var iDoc = fr.contentDocument || fr.contentWindow.document;
            var url = (iDoc.location && iDoc.location.href) || '';
            var isSameOrigin = url.indexOf(host) !== -1 && url !== 'about:blank';
            if (isActive || isSameOrigin) return iDoc;
        } catch(e) {}
    }
    return document; // fallback
}

function injectAfterHeading(doc, headingText, element) {
    var headings = doc.querySelectorAll('h1, h2, h3');
    for (var i = 0; i < headings.length; i++) {
        if (headings[i].textContent.trim().toLowerCase() === headingText.toLowerCase()) {
            headings[i].parentNode.insertBefore(element, headings[i].nextSibling);
            return true; // success
        }
    }
    return false; // not rendered yet — retry
}
```

**Why the injection succeeded:**

1. **Route confirmed via outer page hash** — `window.location.hash === '#/orders'` provided the page signal
2. **Portal DOM accessed via iframe** — `iframe.active-frame.contentDocument` was the correct document, not `document` (the outer Stencil page)
3. **Case-insensitive text matching** — `h3` elements were searched by `textContent.trim().toLowerCase()` rather than MUI CSS class names (which are hash-based and change with builds)
4. **Polling for async render** — B2BE's React renders the `h3` asynchronously after the iframe document is ready. A 300ms polling interval (max 40 attempts = 12s) was required to catch the element after React mounted it
5. **`parentNode.insertBefore(el, heading.nextSibling)`** — inserted the element as the next sibling of the heading, placing it directly below the title

### Injection targets

| Target | Document | Method |
|---|---|---|
| Inside B2BE portal views (headings, tables, etc.) | `iframe.active-frame.contentDocument` | `parentNode.insertBefore()` / `appendChild()` |
| Above the B2BE overlay (outer Stencil page) | `document` | `document.body.insertBefore()` |
| React component context | Not directly possible | Use DOM MutationObserver to detect React-rendered elements and modify adjacent DOM |

### Important: React re-renders

B2BE's React app may re-render components (e.g., after data loads or user interaction), replacing the DOM nodes you injected into. Use a `MutationObserver` on `iDoc.body` to detect when your injected element is removed and re-inject it.

---

## Performance Considerations

- **On non-portal pages** (homepage, PDP, cart): `window.B3` check is synchronous and fast. Once confirmed not a portal page, monitoring can stop entirely.
- **On portal pages**: a 500ms polling init (max 15s) catches `replaceState`, then `hashchange` listener handles navigation — negligible overhead.
- **The 1-second `setInterval` badge approach** used during development should be replaced with event-driven detection in production.
- **`window.b2b.utils`** should be polled for after B2BE channel is confirmed — it appears 1–2s after page load.

---

## Open Questions

1. ~~**What does `window.b2b` expose beyond `utils.user.getB2BToken()`?**~~ **Resolved** — Full SDK surface mapped. See Signal 3 above.
2. **`b2b.callbacks` Map** — Can entries be added to subscribe to B2BE page transition events? If yes, this replaces the `hashchange` + polling approach with a first-class event subscription.
3. **`b2b.utils.getRoutes()`** — What does it return? Could provide a canonical route list and current page without needing hash inspection.
4. **`b2b.utils.openPage(route)`** — What format does `route` accept? Could be used to programmatically navigate between B2BE pages from an external script.
5. **Quick Order page inputs** — do they use the same `input[type="number"][inputmode="numeric"]` selector as the Quote page, or a different DOM structure?
6. **MutationObserver re-injection** — Test whether injected elements survive B2BE React re-renders (e.g., after pagination, sort, or filter actions on list pages).
