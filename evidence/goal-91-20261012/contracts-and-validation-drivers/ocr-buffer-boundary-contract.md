# OCR buffer byte-size boundary correction

Confirmed public LoadImage(1x1 PNG, Height=1<<31, MaxPixels=MaxInt)
passes int area checks but panics at make([]float64, area) because area*8
overflows the platform integer width. Stack has the same shape when two
owned 1x1 inputs use gap=MaxInt/8. Both must return zero result and an error
before allocation. Preserve ordinary custom pixel limits and image behavior.

Root expects one shared checked float64 raster area guard (rasterImageArea)
to reject positive width*height > MaxInt/8 after current multiplication guard.
Stack must use that checked helper instead of its separate final area guard.
Crop, validatePageImage and resizeOCRShape already call the same helper and
inherit the bound. No recover wrapper, runtime memory exhaustion simulation,
new dependencies, arbitrary hard pixel cap, or normal-path data changes.

Tests-only file tasks/ocr/raster_buffer_test.go: public 1x1 PNG LoadImage
with large aspect-preserving Height (1<<((strconv.IntSize-2)/2)) and MaxInt
pixel budget; public Stack of two one-pixel images and gap MaxInt/8.
Both cases must reject with zero result and descriptive overflow/size error.
Catch panic only in tests to turn the existing crash into an actionable failure.
No giant allocations. Root reviews and executes RED, then separate worker
changes only raster.go and page.go, never tests. Existing image/page/resize
tests and real KMNIST/new-process comparison remain required GREEN.
