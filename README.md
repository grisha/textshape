# textshape

A pure Go implementation of text shaping, inspired by [HarfBuzz](https://harfbuzz.github.io/).

## Features

- **OpenType Shaping**: GSUB (substitution) and GPOS (positioning)
- **Ligatures**: Standard ligatures (fi, fl, ffi, ffl, etc.)
- **Kerning**: Pair adjustment positioning
- **Mark positioning**: Base-to-mark, mark-to-mark attachment
- **Font Subsetting**: Create minimal fonts for PDF embedding
- **CFF Support**: OpenType/CFF font subsetting with subroutine optimization
- **HarfBuzz-compatible API**: Similar concepts and data structures

## Installation

```bash
go get github.com/boxesandglue/textshape
```

## Quick Start

```go
package main

import (
    "fmt"
    "os"

    "github.com/boxesandglue/textshape/ot"
)

func main() {
    // Load font
    data, _ := os.ReadFile("Roboto-Regular.ttf")
    font, _ := ot.ParseFont(data, 0)

    // Create shaper
    shaper, _ := ot.NewShaper(font)

    // Shape text
    buf := ot.NewBuffer()
    buf.AddString("office")
    buf.GuessSegmentProperties()
    shaper.Shape(buf, nil) // nil = default features

    // Read results
    for i, info := range buf.Info {
        fmt.Printf("glyph=%d advance=%d\n", info.GlyphID, buf.Pos[i].XAdvance)
    }
}
```

## API Overview

### Buffer

```go
buf := ot.NewBuffer()
buf.AddString("Hello")           // Add text
buf.AddCodepoints([]Codepoint{}) // Or add codepoints directly
buf.GuessSegmentProperties()     // Auto-detect direction/script
buf.SetDirection(ot.DirectionRTL)
buf.Flags = ot.BufferFlagRemoveDefaultIgnorables
```

### Shaper

```go
shaper, err := ot.NewShaper(font)
shaper.Shape(buf, nil)           // Use default features
shaper.Shape(buf, features)      // Use specific features
shaper.ShapeString("text")       // Convenience method
```

### Features

```go
// Create features programmatically
features := []ot.Feature{
    ot.NewFeatureOn(ot.TagLiga),   // Enable ligatures
    ot.NewFeatureOff(ot.TagKern),  // Disable kerning
    ot.NewFeature(ot.TagAalt, 2),  // Alternate #2
}

// Or parse from string (HarfBuzz-compatible syntax)
f, ok := ot.FeatureFromString("kern")      // kern=1
f, ok := ot.FeatureFromString("-liga")     // liga=0
f, ok := ot.FeatureFromString("aalt=2")    // aalt=2
f, ok := ot.FeatureFromString("kern[3:5]") // kern for clusters 3-5

// Parse comma-separated list
features := ot.ParseFeatures("liga,kern,-smcp")
```

### Convenience Function

```go
// One-liner (caches shapers internally)
err := ot.Shape(font, buf, features)
```

### Font Subsetting

```go
import "github.com/boxesandglue/textshape/subset"

// Simple subsetting
result, err := subset.SubsetString(font, "Hello World")

// With options
input := subset.NewInput()
input.AddString("Hello World")
input.Flags = subset.FlagDropLayoutTables // For PDF embedding

plan, _ := subset.CreatePlan(font, input)
result, _ := plan.Execute()
```

## Supported OpenType Features

### GSUB (Glyph Substitution)

| Type | Name | Status |
|------|------|--------|
| 1 | Single Substitution | ✓ |
| 2 | Multiple Substitution | ✓ |
| 3 | Alternate Substitution | ✓ |
| 4 | Ligature Substitution | ✓ |
| 5 | Context Substitution | ✓ |
| 6 | Chaining Context | ✓ |
| 7 | Extension | ✓ |
| 8 | Reverse Chaining | ✓ |

### GPOS (Glyph Positioning)

| Type | Name | Status |
|------|------|--------|
| 1 | Single Adjustment | ✓ |
| 2 | Pair Adjustment (Kerning) | ✓ |
| 3 | Cursive Attachment | ✓ |
| 4 | Mark-to-Base | ✓ |
| 5 | Mark-to-Ligature | ✓ |
| 6 | Mark-to-Mark | ✓ |
| 7 | Context Positioning | ✓ |
| 8 | Chaining Context | ✓ |
| 9 | Extension | ✓ |

## Comparison with HarfBuzz

go-hb produces identical results to HarfBuzz for Latin scripts:

```
Text      HarfBuzz                    go-hb
-----     --------                    -----
Hello     [44 73 80 80 83]           [44 73 80 80 83]    ✓
office    [83 446 71 73]             [83 446 71 73]      ✓
fi        [444]                       [444]               ✓
AV        [37+1249 58+1303]          [37+1249 58+1303]   ✓
```

## Limitations

- **Complex scripts**: Arabic, Indic, Thai shapers not yet implemented
- **Variable fonts**: Not yet supported
- **Graphite**: Not supported

For complex scripts, consider using the full HarfBuzz via cgo or the [textlayout](https://github.com/boxesandglue/textlayout) package.

## Demo Program

```bash
cd cmd/demo
go build -o demo

./demo -font /path/to/font.ttf -text "Hello"
./demo -font /path/to/font.ttf -text "office"
./demo -font /path/to/font.ttf -text "fi" -features "-liga"
```

## License

MIT License - see [LICENSE](LICENSE)

## Acknowledgments

- [HarfBuzz](https://harfbuzz.github.io/) - The original text shaping engine
- [textlayout](https://github.com/boxesandglue/textlayout) - Go port that inspired this implementation
