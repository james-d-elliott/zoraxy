package forward

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type AuthRouterOptions struct {
	// Address of the forward auth endpoint.
	Address string

	// ResponseHeaders is a list of headers to be copied from the response if provided by the forward auth endpoint to
	// the request.
	ResponseHeaders []string

	// ResponseClientHeaders is a list of headers to be copied from the response if provided by the forward auth
	// endpoint to the response to the client.
	ResponseClientHeaders []string

	// RequestHeaders is a list of headers to be copied from the request to the authorization server. If empty all
	// headers are copied.
	RequestHeaders []string

	// RequestIncludedCookies is a list of cookie keys that if defined will be the only cookies sent in the request to
	// the authorization server.
	RequestIncludedCookies []string

	// RequestExcludedCookies is a list of cookie keys that should be removed from every request sent to the upstream.
	RequestExcludedCookies []string

	// RequestIncludeBody enables copying the request body to the request to the authorization server.
	RequestIncludeBody bool

	// UseXOriginalHeaders is a boolean that determines if the X-Original-* headers should be used instead of the
	// X-Forwarded-* headers.
	UseXOriginalHeaders bool

	Logger   *logger.Logger
	Database *database.Database
}

type AuthRouter struct {
	client  *http.Client
	options *AuthRouterOptions
}

// NewAuthRouter creates a new AuthRouter object
func NewAuthRouter(options *AuthRouterOptions) *AuthRouter {
	options.Database.NewTable(DatabaseTable)

	//Read settings from database if available.
	options.Database.Read(DatabaseTable, DatabaseKeyAddress, &options.Address)

	responseHeaders, responseClientHeaders, requestHeaders, requestIncludedCookies, requestExcludedCookies := "", "", "", "", ""

	options.Database.Read(DatabaseTable, DatabaseKeyResponseHeaders, &responseHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyResponseClientHeaders, &responseClientHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestHeaders, &requestHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestIncludedCookies, &requestIncludedCookies)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestExcludedCookies, &requestExcludedCookies)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestIncludeBody, &options.RequestIncludeBody)
	options.Database.Read(DatabaseTable, DatabaseKeyUseXOriginalHeaders, &options.UseXOriginalHeaders)

	options.ResponseHeaders = cleanSplit(responseHeaders)
	options.ResponseClientHeaders = cleanSplit(responseClientHeaders)
	options.RequestHeaders = cleanSplit(requestHeaders)
	options.RequestIncludedCookies = cleanSplit(requestIncludedCookies)
	options.RequestExcludedCookies = cleanSplit(requestExcludedCookies)

	r := &AuthRouter{
		client: &http.Client{
			CheckRedirect: func(r *http.Request, via []*http.Request) (err error) {
				return http.ErrUseLastResponse
			},
		},
		options: options,
	}

	r.logOptions()

	return r
}

// HandleAPIOptions is the internal handler for setting the options.
func (ar *AuthRouter) HandleAPIOptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ar.handleOptionsGET(w, r)
	case http.MethodPost:
		ar.handleOptionsPOST(w, r)
	case http.MethodDelete:
		ar.handleOptionsDelete(w, r)
	default:
		ar.handleOptionsMethodNotAllowed(w, r)
	}
}

func (ar *AuthRouter) handleOptionsGET(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(map[string]any{
		DatabaseKeyAddress:                ar.options.Address,
		DatabaseKeyResponseHeaders:        ar.options.ResponseHeaders,
		DatabaseKeyResponseClientHeaders:  ar.options.ResponseClientHeaders,
		DatabaseKeyRequestHeaders:         ar.options.RequestHeaders,
		DatabaseKeyRequestIncludedCookies: ar.options.RequestIncludedCookies,
		DatabaseKeyRequestExcludedCookies: ar.options.RequestExcludedCookies,
		DatabaseKeyRequestIncludeBody:     ar.options.RequestIncludeBody,
		DatabaseKeyUseXOriginalHeaders:    ar.options.UseXOriginalHeaders,
	})

	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Configuration Options: %s", js), nil)

	utils.SendJSONResponse(w, string(js))

	return
}

func (ar *AuthRouter) handleOptionsPOST(w http.ResponseWriter, r *http.Request) {
	// Update the settings
	address, err := utils.PostPara(r, DatabaseKeyAddress)
	if err != nil {
		utils.SendErrorResponse(w, "address not found")

		return
	}

	// These are optional fields and can be empty strings.
	responseHeaders, _ := utils.PostPara(r, DatabaseKeyResponseHeaders)
	responseClientHeaders, _ := utils.PostPara(r, DatabaseKeyResponseClientHeaders)
	requestHeaders, _ := utils.PostPara(r, DatabaseKeyRequestHeaders)
	requestIncludedCookies, _ := utils.PostPara(r, DatabaseKeyRequestIncludedCookies)
	requestExcludedCookies, _ := utils.PostPara(r, DatabaseKeyRequestExcludedCookies)
	requestIncludeBody, _ := utils.PostPara(r, DatabaseKeyRequestIncludeBody)
	useXOriginalHeaders, _ := utils.PostPara(r, DatabaseKeyUseXOriginalHeaders)

	// Write changes to runtime
	ar.options.Address = address
	ar.options.ResponseHeaders = strings.Split(responseHeaders, ",")
	ar.options.ResponseClientHeaders = strings.Split(responseClientHeaders, ",")
	ar.options.RequestHeaders = strings.Split(requestHeaders, ",")
	ar.options.RequestIncludedCookies = strings.Split(requestIncludedCookies, ",")
	ar.options.RequestExcludedCookies = strings.Split(requestExcludedCookies, ",")
	ar.options.RequestIncludeBody, _ = strconv.ParseBool(requestIncludeBody)
	ar.options.UseXOriginalHeaders, _ = strconv.ParseBool(useXOriginalHeaders)

	// Write changes to database
	ar.options.Database.Write(DatabaseTable, DatabaseKeyAddress, address)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseHeaders, responseHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseClientHeaders, responseClientHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestHeaders, requestHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestIncludedCookies, requestIncludedCookies)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestExcludedCookies, requestExcludedCookies)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestIncludeBody, ar.options.RequestIncludeBody)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyUseXOriginalHeaders, ar.options.UseXOriginalHeaders)

	ar.logOptions()

	utils.SendOK(w)
}

func (ar *AuthRouter) handleOptionsDelete(w http.ResponseWriter, r *http.Request) {
	ar.options.Address = ""
	ar.options.ResponseHeaders = nil
	ar.options.ResponseClientHeaders = nil
	ar.options.RequestHeaders = nil
	ar.options.RequestIncludedCookies = nil
	ar.options.RequestExcludedCookies = nil
	ar.options.RequestIncludeBody = false
	ar.options.UseXOriginalHeaders = false

	ar.options.Database.Delete(DatabaseTable, DatabaseKeyAddress)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyResponseHeaders)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyResponseClientHeaders)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyRequestHeaders)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyRequestIncludedCookies)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyRequestExcludedCookies)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyRequestIncludeBody)
	ar.options.Database.Delete(DatabaseTable, DatabaseKeyUseXOriginalHeaders)

	utils.SendOK(w)
}

func (ar *AuthRouter) handleOptionsMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

	return
}

func (ar *AuthRouter) HandleAuthProviderRoutingRecorded(w http.ResponseWriter, r *http.Request) (err error) {
	buf := new(bytes.Buffer)

	wr := &ResponseRecorder{ResponseWriter: w, Body: buf}

	if err = ar.HandleAuthProviderRouting(wr, r); err != nil {
		dumped, _ := httputil.DumpResponse(&http.Response{StatusCode: wr.Status, Header: wr.Header(), Body: io.NopCloser(buf)}, false)

		ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Response Validation with dump:\n%s", string(dumped)), err)

		return err
	}

	dumped, _ := httputil.DumpResponse(&http.Response{StatusCode: wr.Status, Header: wr.Header(), Body: io.NopCloser(buf)}, false)

	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Response Validation with dump:\n%s", string(dumped)), nil)

	return nil
}

// HandleAuthProviderRouting is the internal handler for Forward Auth authentication.
func (ar *AuthRouter) HandleAuthProviderRouting(w http.ResponseWriter, r *http.Request) error {
	if ar.options.Address == "" {
		return ar.handle500Error(w, nil, "Address not set")
	}

	dumped, _ := httputil.DumpRequest(r, false)

	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Request Validation with dump:\n%s", string(dumped)), nil)

	ar.logOptions()

	// Make a request to Authz Server to verify the request
	req, err := http.NewRequest(http.MethodGet, ar.options.Address, nil)
	if err != nil {
		return ar.handle500Error(w, err, "Unable to create request")
	}

	headerCopyIncluded(r.Header, req.Header, ar.options.RequestHeaders, true)
	headerCookieRedact(r, ar.options.RequestIncludedCookies, false)

	// TODO: Add support for headers from upstream proxies. This will likely involve implementing some form of
	//       proxy specific trust system within Zoraxy.
	if ar.options.UseXOriginalHeaders {
		rSetXOriginalHeaders(r, req)
	} else {
		rSetXForwardedHeaders(r, req)
	}

	if ar.options.RequestIncludeBody {
		if err = rCopyBody(r, req); err != nil {
			return ar.handle500Error(w, err, "Unable to perform forwarded auth due to a request copy error")
		}
	}

	dumped, _ = httputil.DumpRequestOut(req, false)

	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Rquest to %s with dump:\n%s", ar.options.Address, string(dumped)), nil)

	// Make the Authz Request.
	respForwarded, err := ar.client.Do(req)
	if err != nil {
		return ar.handle500Error(w, err, "Unable to perform forwarded auth due to a request error")
	}

	defer respForwarded.Body.Close()

	dumped, _ = httputil.DumpResponse(respForwarded, false)

	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Response from %s with dump:\n%s", ar.options.Address, string(dumped)), nil)

	// Responses within the 200-299 range are considered successful and allow the proxy to handle the request.
	if respForwarded.StatusCode >= http.StatusOK && respForwarded.StatusCode < http.StatusMultipleChoices {
		if len(ar.options.ResponseClientHeaders) != 0 {
			headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseClientHeaders, false)
		}

		headerCookieRedact(r, ar.options.RequestExcludedCookies, true)

		if len(ar.options.ResponseHeaders) != 0 {
			// Copy specific user-specified headers from the response of the forward auth request to the request sent to the
			// upstream server/next hop.
			headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseHeaders, false)
		}

		// Return the request to the proxy for forwarding to the backend.
		return nil
	}

	// Copy the unsuccessful response.
	headerCopyExcluded(respForwarded.Header, w.Header(), nil)

	if ar.options.UseXOriginalHeaders && respForwarded.StatusCode == 401 && respForwarded.Header.Get(HeaderLocation) != "" {
		w.WriteHeader(http.StatusFound)
	} else {
		w.WriteHeader(respForwarded.StatusCode)
	}

	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Response from %s with dump: %s", ar.options.Address, string(dumped)), nil)

	if _, err = io.Copy(w, respForwarded.Body); err != nil {
		return ar.handle500Error(w, err, "Unable to copy response")
	}

	return ErrUnauthorized
}

// handle500Error is func intended on factorizing a commonly repeated functional flow within this provider.
func (ar *AuthRouter) handle500Error(w http.ResponseWriter, err error, message string) error {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

	ar.options.Logger.PrintAndLog(LogTitle, message, err)

	return ErrInternalServerError
}

func (ar *AuthRouter) logOptions() {
	ar.options.Logger.PrintAndLog(LogTitle, fmt.Sprintf("Forward Authz Options -> Address: %s, Response Headers: %s, Response Client Headers: %s, Request Headers: %s, Request Included Cookies: %s, Request Excluded Cookies: %s, Request Include Body: %t, Use X-Original Headers: %t", ar.options.Address, strings.Join(ar.options.ResponseHeaders, ";"), strings.Join(ar.options.ResponseClientHeaders, ";"), strings.Join(ar.options.RequestHeaders, ";"), strings.Join(ar.options.RequestIncludedCookies, ";"), strings.Join(ar.options.RequestExcludedCookies, ";"), ar.options.RequestIncludeBody, ar.options.UseXOriginalHeaders), nil)
}

type ResponseRecorder struct {
	http.ResponseWriter
	Status int
	Body   *bytes.Buffer
}

func (rr *ResponseRecorder) Header() http.Header {
	return rr.ResponseWriter.Header()
}

func (rr *ResponseRecorder) Write(b []byte) (int, error) {
	rr.Body.Write(b)                  // Capture the body
	return rr.ResponseWriter.Write(b) // Write to the original ResponseWriter
}

func (rr *ResponseRecorder) WriteHeader(statusCode int) {
	rr.Status = statusCode // Capture the status code
	rr.ResponseWriter.WriteHeader(statusCode)
}
