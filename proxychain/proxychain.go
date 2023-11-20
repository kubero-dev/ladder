package proxychain

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"ladder/pkg/ruleset"

	"github.com/gofiber/fiber/v2"
)

/*
ProxyChain manages the process of forwarding an HTTP request to an upstream server,
applying request and response modifications along the way.

  - It accepts incoming HTTP requests (as a Fiber *ctx), and applies
    request modifiers (ReqMods) and response modifiers (ResMods) before passing the
    upstream response back to the client.

  - ProxyChains can be reused to avoid memory allocations. However, they are not concurrent-safe
    so a ProxyChainPool should be used with mutexes to avoid memory errors.

---

# EXAMPLE

```

import (

	rx "ladder/pkg/proxychain/requestmodifers"
	tx "ladder/pkg/proxychain/responsemodifers"
	"ladder/internal/proxychain"

)

proxychain.NewProxyChain().

	SetFiberCtx(c).
	SetRequestModifications(
		rx.BlockOutgoingCookies(),
		rx.SpoofOrigin(),
		rx.SpoofReferrer(),
	).
	SetResultModifications(
		tx.BlockIncomingCookies(),
		tx.RewriteHTMLResourceURLs()
	).
	Execute()

```

	client            ladder service            upstream

┌─────────┐    ┌────────────────────────┐    ┌─────────┐
│         │GET │                        │    │         │
│       req────┼───► ProxyChain         │    │         │
│         │    │       │                │    │         │
│         │    │       ▼                │    │         │
│         │    │     apply              │    │         │
│         │    │ RequestModifications   │    │         │
│         │    │       │                │    │         │
│         │    │       ▼                │    │         │
│         │    │     send        GET    │    │         │
│         │    │     Request req────────┼─►  │         │
│         │    │                        │    │         │
│         │    │                 200 OK │    │         │
│         │    │       ┌────────────────┼─response     │
│         │    │       ▼                │    │         │
│         │    │     apply              │    │         │
│         │    │ ResultModifications    │    │         │
│         │    │       │                │    │         │
│         │◄───┼───────┘                │    │         │
│         │    │ 200 OK                 │    │         │
│         │    │                        │    │         │
└─────────┘    └────────────────────────┘    └─────────┘
*/
type ProxyChain struct {
	Context              *fiber.Ctx
	Client               *http.Client
	Request              *http.Request
	Response             *http.Response
	requestModifications []RequestModification
	resultModifications  []ResponseModification
	Ruleset              *ruleset.RuleSet
	debugMode            bool
	abortErr             error
}

// a ProxyStrategy is a pre-built proxychain with purpose-built defaults
type ProxyStrategy ProxyChain

// A RequestModification is a function that should operate on the
// ProxyChain Req or Client field, using the fiber ctx as needed.
type RequestModification func(*ProxyChain) error

// A ResponseModification is a function that should operate on the
// ProxyChain Res (http result) & Body (buffered http response body) field
type ResponseModification func(*ProxyChain) error

// SetRequestModifications sets the ProxyChain's request modifers
// the modifier will not fire until ProxyChain.Execute() is run.
func (chain *ProxyChain) SetRequestModifications(mods ...RequestModification) *ProxyChain {
	chain.requestModifications = mods
	return chain
}

// AddRequestModifications sets the ProxyChain's request modifers
// the modifier will not fire until ProxyChain.Execute() is run.
func (chain *ProxyChain) AddRequestModifications(mods ...RequestModification) *ProxyChain {
	chain.requestModifications = append(chain.requestModifications, mods...)
	return chain
}

// AddResponseModifications sets the ProxyChain's response modifers
// the modifier will not fire until ProxyChain.Execute() is run.
func (chain *ProxyChain) AddResponseModifications(mods ...ResponseModification) *ProxyChain {
	chain.resultModifications = mods
	return chain
}

// Adds a ruleset to ProxyChain
func (chain *ProxyChain) AddRuleset(rs *ruleset.RuleSet) *ProxyChain {
	chain.Ruleset = rs
	// TODO: add _applyRuleset method
	return chain
}

func (chain *ProxyChain) _initialize_request() (*http.Request, error) {
	if chain.Context == nil {
		chain.abortErr = chain.abort(errors.New("no context set"))
		return nil, chain.abortErr
	}
	// initialize a request (without url)
	req, err := http.NewRequest(chain.Context.Method(), "", nil)
	if err != nil {
		return nil, err
	}
	chain.Request = req
	switch chain.Context.Method() {
	case "GET":
	case "DELETE":
	case "HEAD":
	case "OPTIONS":
		break
	case "POST":
	case "PUT":
	case "PATCH":
		// stream content of body from client request to upstream request
		chain.Request.Body = io.NopCloser(chain.Context.Request().BodyStream())
	default:
		return nil, fmt.Errorf("unsupported request method from client: '%s'", chain.Context.Method())
	}

	/*
		// copy client request headers to upstream request headers
		forwardHeaders := func(key []byte, val []byte) {
			req.Header.Set(string(key), string(val))
		}
		clientHeaders := &chain.Context.Request().Header
		clientHeaders.VisitAll(forwardHeaders)
	*/

	return req, nil
}

// _execute sends the request for the ProxyChain and returns the raw body only
// the caller is responsible for returning a response back to the requestor
// the caller is also responsible for calling chain._reset() when they are done with the body
func (chain *ProxyChain) _execute() (io.Reader, error) {
	if chain.validateCtxIsSet() != nil || chain.abortErr != nil {
		return nil, chain.abortErr
	}
	if chain.Request == nil {
		return nil, errors.New("proxychain request not yet initialized")
	}
	if chain.Request.URL.Scheme == "" {
		return nil, errors.New("request url not set or invalid. Check ProxyChain ReqMods for issues")
	}

	// Apply requestModifications to proxychain
	for _, applyRequestModificationsTo := range chain.requestModifications {
		err := applyRequestModificationsTo(chain)
		if err != nil {
			return nil, chain.abort(err)
		}
	}

	// Send Request Upstream
	resp, err := chain.Client.Do(chain.Request)
	if err != nil {
		return nil, chain.abort(err)
	}
	chain.Response = resp

	//defer resp.Body.Close()

	/* todo: move to rsm
	for k, v := range resp.Header {
		chain.Context.Set(k, resp.Header.Get(k))
	}
	*/

	// Apply ResponseModifiers to proxychain
	for _, applyResultModificationsTo := range chain.resultModifications {
		err := applyResultModificationsTo(chain)
		if err != nil {
			return nil, chain.abort(err)
		}
	}

	return chain.Response.Body, nil
}

// Execute sends the request for the ProxyChain and returns the request to the sender
// and resets the fields so that the ProxyChain can be reused.
// if any step in the ProxyChain fails, the request will abort and a 500 error will
// be returned to the client
func (chain *ProxyChain) Execute() error {
	defer chain._reset()
	body, err := chain._execute()
	if err != nil {
		log.Println(err)
		return err
	}
	if chain.Context == nil {
		return errors.New("no context set")
	}
	// Return request back to client
	chain.Context.Set("content-type", chain.Response.Header.Get("content-type"))
	return chain.Context.SendStream(body)

	//return chain.Context.SendStream(body)
}

// reconstructUrlFromReferer reconstructs the URL using the referer's scheme, host, and the relative path / queries
func reconstructUrlFromReferer(referer *url.URL, relativeUrl *url.URL) (*url.URL, error) {

	// Extract the real url from referer path
	realUrl, err := url.Parse(strings.TrimPrefix(referer.Path, "/"))
	if err != nil {
		return nil, fmt.Errorf("error parsing real URL from referer '%s': %v", referer.Path, err)
	}

	if realUrl.Scheme == "" || realUrl.Host == "" {
		return nil, fmt.Errorf("invalid referer URL: '%s' on request '%s", referer, relativeUrl)
	}

	log.Printf("'%s' -> '%s'\n", relativeUrl.String(), realUrl.String())

	return &url.URL{
		Scheme:   referer.Scheme,
		Host:     referer.Host,
		Path:     realUrl.Path,
		RawQuery: realUrl.RawQuery,
	}, nil
}

// extractUrl extracts a URL from the request ctx. If the URL in the request
// is a relative path, it reconstructs the full URL using the referer header.
func (chain *ProxyChain) extractUrl() (*url.URL, error) {
	// try to extract url-encoded
	reqUrl, err := url.QueryUnescape(chain.Context.Params("*"))
	if err != nil {
		reqUrl = chain.Context.Params("*") // fallback
	}

	urlQuery, err := url.Parse(reqUrl)
	if err != nil {
		return nil, fmt.Errorf("error parsing request URL '%s': %v", reqUrl, err)
	}

	// Handle standard paths
	// eg: https://localhost:8080/https://realsite.com/images/foobar.jpg -> https://realsite.com/images/foobar.jpg
	isRelativePath := urlQuery.Scheme == ""
	if !isRelativePath {
		return urlQuery, nil
	}

	// Handle relative URLs
	// eg: https://localhost:8080/images/foobar.jpg -> https://realsite.com/images/foobar.jpg
	referer, err := url.Parse(chain.Context.Get("referer"))
	relativePath := urlQuery
	if err != nil {
		return nil, fmt.Errorf("error parsing referer URL from req: '%s': %v", relativePath, err)
	}
	return reconstructUrlFromReferer(referer, relativePath)
}

// SetFiberCtx takes the request ctx from the client
// for the modifiers and execute function to use.
// it must be set everytime a new request comes through
// if the upstream request url cannot be extracted from the ctx,
// a 500 error will be sent back to the client
func (chain *ProxyChain) SetFiberCtx(ctx *fiber.Ctx) *ProxyChain {
	chain.Context = ctx

	// initialize the request and prepare it for modification
	req, err := chain._initialize_request()
	if err != nil {
		chain.abortErr = chain.abort(err)
	}
	chain.Request = req

	// extract the URL for the request and add it to the new request
	url, err := chain.extractUrl()
	if err != nil {
		chain.abortErr = chain.abort(err)
	}
	chain.Request.URL = url
	fmt.Printf("extracted URL: %s\n", chain.Request.URL)

	return chain
}

func (chain *ProxyChain) validateCtxIsSet() error {
	if chain.Context != nil {
		return nil
	}
	err := errors.New("proxyChain was called without setting a fiber Ctx. Use ProxyChain.SetCtx()")
	chain.abortErr = chain.abort(err)
	return chain.abortErr
}

// SetHttpClient sets a new upstream http client transport
// useful for modifying TLS
func (chain *ProxyChain) SetHttpClient(httpClient *http.Client) *ProxyChain {
	chain.Client = httpClient
	return chain
}

// SetVerbose changes the logging behavior to print
// the modification steps and applied rulesets for debugging
func (chain *ProxyChain) SetDebugLogging(isDebugMode bool) *ProxyChain {
	chain.debugMode = isDebugMode
	return chain
}

// abort proxychain and return 500 error to client
// this will prevent Execute from firing and reset the state
// returns the initial error enriched with context
func (chain *ProxyChain) abort(err error) error {
	//defer chain._reset()
	chain.abortErr = err
	chain.Context.Response().SetStatusCode(500)
	e := fmt.Errorf("ProxyChain error for '%s': %s", chain.Request.URL.String(), err.Error())
	chain.Context.SendString(e.Error())
	log.Println(e.Error())
	return e
}

// internal function to reset state of ProxyChain for reuse
func (chain *ProxyChain) _reset() {
	chain.abortErr = nil
	chain.Request = nil
	//chain.Response = nil
	chain.Context = nil
}

// NewProxyChain initializes a new ProxyChain
func NewProxyChain() *ProxyChain {
	chain := new(ProxyChain)
	chain.Client = http.DefaultClient
	return chain
}
