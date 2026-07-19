package xhttp

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/sagernet/sing-box/common/xray/buf"
	"github.com/sagernet/sing-box/common/xray/utils"
	"github.com/sagernet/sing-box/option"
)

func FillStreamRequest(request *http.Request, sessionId string, seqStr string, options *option.V2RayXHTTPBaseOptions) {
	request.Header = options.GetRequestHeader()
	length := int(options.GetNormalizedXPaddingBytes().Rand())
	config := XPaddingConfig{Length: length}
	if options.XPaddingObfsMode {
		config.Placement = XPaddingPlacement{
			Placement: options.XPaddingPlacement,
			Key:       options.XPaddingKey,
			Header:    options.XPaddingHeader,
			RawURL:    request.URL.String(),
		}
		config.Method = PaddingMethod(options.XPaddingMethod)
	} else {
		config.Placement = XPaddingPlacement{
			Placement: option.PlacementQueryInHeader,
			Key:       "x_padding",
			Header:    "Referer",
			RawURL:    request.URL.String(),
		}
	}
	ApplyXPaddingToRequest(request, config)
	ApplyMetaToRequest(options, request, sessionId, "")
	if request.Body != nil && !options.NoGRPCHeader { // stream-up/one
		request.Header.Set("Content-Type", "application/grpc")
	}
}

func FillPacketRequest(request *http.Request, sessionId string, seqStr string, payload buf.MultiBuffer, options *option.V2RayXHTTPBaseOptions) error {
	dataPlacement := options.GetNormalizedUplinkDataPlacement()
	if dataPlacement == option.PlacementBody || dataPlacement == option.PlacementAuto {
		request.Header = options.GetRequestHeader()
		request.Body = io.NopCloser(&buf.MultiBufferContainer{MultiBuffer: payload})
		request.ContentLength = int64(payload.Len())
	} else {
		data := make([]byte, payload.Len())
		payload.Copy(data)
		buf.ReleaseMulti(payload)
		switch dataPlacement {
		case option.PlacementHeader:
			request.Header = GetRequestHeaderWithPayload(data, options)
		case option.PlacementCookie:
			request.Header = options.GetRequestHeader()
			for _, cookie := range GetRequestCookiesWithPayload(data, options) {
				request.AddCookie(cookie)
			}
		}
	}
	length := int(options.GetNormalizedXPaddingBytes().Rand())
	config := XPaddingConfig{Length: length}
	if options.XPaddingObfsMode {
		config.Placement = XPaddingPlacement{
			Placement: options.XPaddingPlacement,
			Key:       options.XPaddingKey,
			Header:    options.XPaddingHeader,
			RawURL:    request.URL.String(),
		}
		config.Method = PaddingMethod(options.XPaddingMethod)
	} else {
		config.Placement = XPaddingPlacement{
			Placement: option.PlacementQueryInHeader,
			Key:       "x_padding",
			Header:    "Referer",
			RawURL:    request.URL.String(),
		}
	}
	ApplyXPaddingToRequest(request, config)
	ApplyMetaToRequest(options, request, sessionId, seqStr)
	return nil
}

func WriteResponseHeader(writer http.ResponseWriter, requestMethod string, requestHeader http.Header, options *option.V2RayXHTTPOptions) {
	// CORS headers for the browser dialer
	if origin := requestHeader.Get("Origin"); origin == "" {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		// Chrome says: The value of the 'Access-Control-Allow-Origin' header in the response must not be the wildcard '*' when the request's credentials mode is 'include'.
		writer.Header().Set("Access-Control-Allow-Origin", origin)
	}
	if options.GetNormalizedSessionPlacement() == option.PlacementCookie ||
		options.GetNormalizedSeqPlacement() == option.PlacementCookie ||
		options.XPaddingPlacement == option.PlacementCookie ||
		options.GetNormalizedUplinkDataPlacement() == option.PlacementCookie {
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	if requestMethod == "OPTIONS" {
		requestedMethod := requestHeader.Get("Access-Control-Request-Method")
		if requestedMethod != "" {
			writer.Header().Set("Access-Control-Allow-Methods", requestedMethod)
		} else {
			writer.Header().Set("Access-Control-Allow-Methods", "*")
		}
		requestedHeaders := requestHeader.Get("Access-Control-Request-Headers")
		if requestedHeaders == "" {
			writer.Header().Set("Access-Control-Allow-Headers", "*")
		} else {
			writer.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
		}
	}
}

func GetRequestHeader(options *option.V2RayXHTTPBaseOptions) http.Header {
	header := http.Header{}
	for k, v := range options.Headers {
		header.Add(k, v)
	}
	utils.TryDefaultHeadersWith(header, "fetch")
	return header
}

func GetRequestHeaderWithPayload(payload []byte, options *option.V2RayXHTTPBaseOptions) http.Header {
	header := GetRequestHeader(options)
	key := options.UplinkDataKey
	encodedData := base64.RawURLEncoding.EncodeToString(payload)
	for i := 0; len(encodedData) > 0; i++ {
		chunkSize := min(int(options.GetNormalizedUplinkChunkSize().Rand()), len(encodedData))
		chunk := encodedData[:chunkSize]
		encodedData = encodedData[chunkSize:]
		headerKey := fmt.Sprintf("%s-%d", key, i)
		header.Set(headerKey, chunk)
	}

	return header
}

func GetRequestCookiesWithPayload(payload []byte, options *option.V2RayXHTTPBaseOptions) []*http.Cookie {
	cookies := []*http.Cookie{}
	key := options.UplinkDataKey
	encodedData := base64.RawURLEncoding.EncodeToString(payload)
	for i := 0; len(encodedData) > 0; i++ {
		chunkSize := min(int(options.GetNormalizedUplinkChunkSize().Rand()), len(encodedData))
		chunk := encodedData[:chunkSize]
		encodedData = encodedData[chunkSize:]
		cookieName := fmt.Sprintf("%s_%d", key, i)
		cookies = append(cookies, &http.Cookie{Name: cookieName, Value: chunk})
	}
	return cookies
}
