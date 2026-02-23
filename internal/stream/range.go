package stream

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type ByteRange struct {
	Start int64
	End   int64
	Valid bool
}

func ParseRangeHeader(header string, size int64) (ByteRange, error) {
	if header == "" {
		return ByteRange{Start: 0, End: size - 1, Valid: false}, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return ByteRange{}, fmt.Errorf("invalid range")
	}
	parts := strings.Split(strings.TrimPrefix(header, "bytes="), "-")
	if len(parts) != 2 {
		return ByteRange{}, fmt.Errorf("invalid range parts")
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return ByteRange{}, err
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return ByteRange{}, err
		}
	}
	if start > end || start < 0 || end >= size {
		return ByteRange{}, fmt.Errorf("invalid range bounds")
	}
	return ByteRange{Start: start, End: end, Valid: true}, nil
}

func WriteRangeHeaders(w http.ResponseWriter, rng ByteRange, size int64, contentType string) {
	w.Header().Set("Accept-Ranges", "bytes")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if rng.Valid {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rng.Start, rng.End, size))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", rng.End-rng.Start+1))
		w.WriteHeader(http.StatusPartialContent)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.WriteHeader(http.StatusOK)
}
