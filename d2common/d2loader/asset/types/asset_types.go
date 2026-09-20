package types

import "strings"

// AssetType represents the type of an asset
type AssetType int

// Asset types
const (
	AssetTypeUnknown AssetType = iota
	AssetTypeJSON
	AssetTypeStringTable
	AssetTypeDataDictionary
	AssetTypePalette
	AssetTypePaletteTransform
	AssetTypeCOF
	AssetTypeDC6
	AssetTypeDCC
	AssetTypeDS1
	AssetTypeDT1
	AssetTypeWAV
	AssetTypeD2

	// AssetTypePNG is the modern sprite format (Plan §5, M5.1). It is appended
	// rather than inserted: the constants are iota-valued and nothing persists
	// them today, but a renumbering is the kind of change that breaks something
	// quietly a year later, so new types go on the end.
	AssetTypePNG

	// AssetTypeOGG is the modern audio format. Reserved here with the sprite
	// type so the pair is visible together; the audio provider does not branch
	// on asset types at all yet (it hands raw bytes to ebiten's wav decoder), so
	// this constant has no consumer until that changes.
	AssetTypeOGG
)

// Ext2AssetType determines the AssetType with the given file extension
func Ext2AssetType(ext string) AssetType {
	ext = strings.ToLower(ext)
	ext = strings.ReplaceAll(ext, ".", "")

	lookup := map[string]AssetType{
		"json": AssetTypeJSON,
		"tbl":  AssetTypeStringTable,
		"txt":  AssetTypeDataDictionary,
		"dat":  AssetTypePalette,
		"pl2":  AssetTypePaletteTransform,
		"cof":  AssetTypeCOF,
		"dc6":  AssetTypeDC6,
		"dcc":  AssetTypeDCC,
		"ds1":  AssetTypeDS1,
		"dt1":  AssetTypeDT1,
		"wav":  AssetTypeWAV,
		"d2":   AssetTypeD2,
		"png":  AssetTypePNG,
		"ogg":  AssetTypeOGG,
	}

	if knownType, found := lookup[ext]; found {
		return knownType
	}

	return AssetTypeUnknown
}
