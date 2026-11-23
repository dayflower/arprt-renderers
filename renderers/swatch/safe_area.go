package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Insets are expressed in upright logical pixels, independent of output rotation.
type safeAreaInsets struct {
	Top    int `json:"top"`
	Bottom int `json:"bottom"`
	Left   int `json:"left"`
	Right  int `json:"right"`
}

// Reject null or noninteger edges while retaining unknown-field compatibility.
func (i *safeAreaInsets) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("safe area must be an object")
	}
	type plain safeAreaInsets
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, edge := range []string{"top", "bottom", "left", "right"} {
		if raw, ok := fields[edge]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("safe area edges must be integers")
		}
	}
	*i = safeAreaInsets(value)
	return nil
}

func (c renderContext) contentSize() (int, int) {
	w, h := c.logicalSize()
	i := c.Target.SafeAreaInsets
	return w - i.Left - i.Right, h - i.Top - i.Bottom
}

func (c renderContext) validateSafeArea() error {
	i := c.Target.SafeAreaInsets
	for _, v := range []int{i.Top, i.Bottom, i.Left, i.Right} {
		if v < 0 || v > 4095 {
			return errors.New("invalid safe area inset")
		}
	}
	w, h := c.contentSize()
	if w <= 0 || h <= 0 {
		return errors.New("safe area must have positive dimensions")
	}
	return nil
}

// layoutHTML preserves the full capture viewport and existing design padding.
// Layout scripts measure this inset body, so fitting uses the same content area.
func (c renderContext) layoutHTML(body []byte) ([]byte, error) {
	if err := c.validateSafeArea(); err != nil {
		return nil, err
	}
	i := c.Target.SafeAreaInsets
	if i != (safeAreaInsets{}) {
		w, h := c.contentSize()
		style := fmt.Sprintf("body{position:absolute;left:%dpx;top:%dpx;width:%dpx!important;height:%dpx!important;overflow:hidden}</style>", i.Left, i.Top, w, h)
		body = bytes.Replace(body, []byte("</style>"), []byte(style), 1)
	}
	if len(body) > htmlLimit {
		return nil, errors.New("HTML exceeds size limit")
	}
	return body, nil
}
