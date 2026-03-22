package gosurfer

import (
	"fmt"

	"github.com/go-rod/rod/lib/proto"
)

// SetViewport overrides the page viewport dimensions and device scale factor.
//
//	page.SetViewport(375, 812, 3.0, true) // iPhone X
func (p *Page) SetViewport(width, height int, scaleFactor float64, mobile bool) error {
	return proto.EmulationSetDeviceMetricsOverride{
		Width:             width,
		Height:            height,
		DeviceScaleFactor: scaleFactor,
		Mobile:            mobile,
	}.Call(p.rod)
}

// SetUserAgent overrides the browser user agent string.
func (p *Page) SetUserAgent(userAgent string) error {
	return proto.EmulationSetUserAgentOverride{
		UserAgent: userAgent,
	}.Call(p.rod)
}

// SetGeolocation sets the geographic location for the page.
// Pass accuracy in meters (e.g., 100.0 for ~100m accuracy).
//
//	page.SetGeolocation(37.7749, -122.4194, 100) // San Francisco
func (p *Page) SetGeolocation(latitude, longitude, accuracy float64) error {
	return proto.EmulationSetGeolocationOverride{
		Latitude:  &latitude,
		Longitude: &longitude,
		Accuracy:  &accuracy,
	}.Call(p.rod)
}

// ClearGeolocation removes the geolocation override.
func (p *Page) ClearGeolocation() error {
	return proto.EmulationClearGeolocationOverride{}.Call(p.rod)
}

// SetTimezone overrides the browser timezone.
// Use IANA timezone identifiers like "America/New_York", "Europe/London", "Asia/Tokyo".
func (p *Page) SetTimezone(timezoneID string) error {
	return proto.EmulationSetTimezoneOverride{
		TimezoneID: timezoneID,
	}.Call(p.rod)
}

// SetLocale overrides the browser locale for Intl APIs (number formatting, date formatting, etc.).
// Use ICU locale format like "en_US", "fr_FR", "ja_JP".
// Note: this affects Intl.DateTimeFormat, Intl.NumberFormat, etc. but not navigator.language.
func (p *Page) SetLocale(locale string) error {
	return proto.EmulationSetLocaleOverride{
		Locale: locale,
	}.Call(p.rod)
}

// SetOffline simulates offline/online network conditions.
//
//	page.SetOffline(true)  // disconnect
//	page.SetOffline(false) // reconnect
func (p *Page) SetOffline(offline bool) error {
	return proto.NetworkEmulateNetworkConditions{
		Offline:            offline,
		Latency:            0,
		DownloadThroughput: -1,
		UploadThroughput:   -1,
	}.Call(p.rod)
}

// SetNetworkConditions emulates network throttling.
// Latency is in milliseconds, throughput values are in bytes/second.
//
//	page.SetNetworkConditions(150, 1.6*1024*1024, 750*1024) // Regular 3G
func (p *Page) SetNetworkConditions(latencyMs float64, downloadBytesPerSec, uploadBytesPerSec float64) error {
	return proto.NetworkEmulateNetworkConditions{
		Offline:            false,
		Latency:            latencyMs,
		DownloadThroughput: downloadBytesPerSec,
		UploadThroughput:   uploadBytesPerSec,
	}.Call(p.rod)
}

// SetTouchEnabled enables or disables touch event emulation.
func (p *Page) SetTouchEnabled(enabled bool) error {
	return proto.EmulationSetTouchEmulationEnabled{
		Enabled: enabled,
	}.Call(p.rod)
}

// ColorScheme represents a CSS color scheme preference.
type ColorScheme string

const (
	ColorSchemeLight        ColorScheme = "light"
	ColorSchemeDark         ColorScheme = "dark"
	ColorSchemeNoPreference ColorScheme = ""
)

// SetColorScheme emulates a CSS prefers-color-scheme media feature.
//
//	page.SetColorScheme(gosurfer.ColorSchemeDark)
func (p *Page) SetColorScheme(scheme ColorScheme) error {
	features := []*proto.EmulationMediaFeature{
		{Name: "prefers-color-scheme", Value: string(scheme)},
	}
	if scheme == ColorSchemeNoPreference {
		features = nil
	}
	return proto.EmulationSetEmulatedMedia{
		Features: features,
	}.Call(p.rod)
}

// ReducedMotion represents a CSS reduced-motion preference.
type ReducedMotion string

const (
	ReducedMotionReduce       ReducedMotion = "reduce"
	ReducedMotionNoPreference ReducedMotion = ""
)

// SetReducedMotion emulates a CSS prefers-reduced-motion media feature.
func (p *Page) SetReducedMotion(motion ReducedMotion) error {
	features := []*proto.EmulationMediaFeature{
		{Name: "prefers-reduced-motion", Value: string(motion)},
	}
	if motion == ReducedMotionNoPreference {
		features = nil
	}
	return proto.EmulationSetEmulatedMedia{
		Features: features,
	}.Call(p.rod)
}

// GrantPermissions grants browser permissions for the given origin.
// Common permissions: "geolocation", "notifications", "camera", "microphone",
// "clipboard-read", "clipboard-write".
func (b *Browser) GrantPermissions(origin string, permissions ...string) error {
	perms := make([]proto.BrowserPermissionType, len(permissions))
	for i, p := range permissions {
		perms[i] = proto.BrowserPermissionType(p)
	}
	return proto.BrowserGrantPermissions{
		Permissions: perms,
		Origin:      origin,
	}.Call(b.rod)
}

// ResetPermissions resets all permission overrides.
func (b *Browser) ResetPermissions() error {
	return proto.BrowserResetPermissions{}.Call(b.rod)
}

// Device represents a pre-configured device profile for emulation.
type Device struct {
	Name      string
	UserAgent string
	Width     int
	Height    int
	Scale     float64
	Mobile    bool
	Touch     bool
}

// EmulateDevice applies a device profile to the page (viewport, user agent, touch).
//
//	page.EmulateDevice(gosurfer.DeviceIPhoneX)
func (p *Page) EmulateDevice(d Device) error {
	if err := p.SetViewport(d.Width, d.Height, d.Scale, d.Mobile); err != nil {
		return fmt.Errorf("gosurfer: emulate device viewport: %w", err)
	}
	if err := p.SetUserAgent(d.UserAgent); err != nil {
		return fmt.Errorf("gosurfer: emulate device user agent: %w", err)
	}
	if d.Touch {
		if err := p.SetTouchEnabled(true); err != nil {
			return fmt.Errorf("gosurfer: emulate device touch: %w", err)
		}
	}
	return nil
}

// Pre-configured device profiles.
var (
	DeviceIPhoneX = Device{
		Name:      "iPhone X",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		Width:     375,
		Height:    812,
		Scale:     3.0,
		Mobile:    true,
		Touch:     true,
	}
	DeviceIPhone14Pro = Device{
		Name:      "iPhone 14 Pro",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		Width:     393,
		Height:    852,
		Scale:     3.0,
		Mobile:    true,
		Touch:     true,
	}
	DevicePixel7 = Device{
		Name:      "Pixel 7",
		UserAgent: "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
		Width:     412,
		Height:    915,
		Scale:     2.625,
		Mobile:    true,
		Touch:     true,
	}
	DeviceIPadPro = Device{
		Name:      "iPad Pro 12.9",
		UserAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		Width:     1024,
		Height:    1366,
		Scale:     2.0,
		Mobile:    true,
		Touch:     true,
	}
	DeviceGalaxyS23 = Device{
		Name:      "Galaxy S23",
		UserAgent: "Mozilla/5.0 (Linux; Android 14; SM-S911B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
		Width:     360,
		Height:    780,
		Scale:     3.0,
		Mobile:    true,
		Touch:     true,
	}
	DeviceDesktop1080p = Device{
		Name:      "Desktop 1080p",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Width:     1920,
		Height:    1080,
		Scale:     1.0,
		Mobile:    false,
		Touch:     false,
	}
	DeviceDesktop4K = Device{
		Name:      "Desktop 4K",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Width:     3840,
		Height:    2160,
		Scale:     2.0,
		Mobile:    false,
		Touch:     false,
	}
)
