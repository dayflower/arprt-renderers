# Picsum changelog

## 0.2.0

- Replace the Python/Pillow runtime with a static Go binary in a minimal
  `scratch` image containing HTTPS root certificates and license notices.
- Preserve the CLI, Render Context v1, gray4/bw/rgb modes, clockwise rotations,
  JPEG EXIF orientation, Lanczos fitting, autocontrast and Floyd-Steinberg
  dithering. Preserve the exact 2-bit grayscale PNG format, exclusive 90,000-byte
  grayscale budget and undithered fallback, bounded downloads and atomic output
  replacement.
- Image pixels and compressed bytes can differ from Pillow due to resampling,
  quantization and compression implementation differences. Local inputs support
  JPEG, PNG, GIF (first frame), TIFF, BMP and WebP; other Pillow-specific formats
  are no longer supported.
- Update development tooling and native amd64/arm64 CI for Go. Compile a separate
  test binary and mount it read-only into the production image to verify Job
  security restrictions without shipping a compiler or test suite.

See the GitHub release for the immutable image reference, source revision and
the validation actually performed. Live Picsum, Kubernetes integration and
physical display acceptance are outside the automated release checks.

## 0.1.0

- Initial independent Picsum renderer release, imported from ARPRT v0.2.2 using
  Python/Pillow, with native Linux amd64 and arm64 images and offline Job checks.
