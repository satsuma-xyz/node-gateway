package util

import (
	"bytes"
	"io"
	"net/http"
)

func ReadAndCopyBackResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return respBody, err
	}

	resp.Body = io.NopCloser(bytes.NewBuffer(respBody))

	return respBody, nil
}
