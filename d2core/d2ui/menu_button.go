package d2ui

import (
	"image"
	"image/color"
	"math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
)

// NewMenuButton uses the normal button input path with original, code-drawn
// faces. It loads no inherited button artwork. Dimensions are logical UI pixels.
func (ui *UIManager) NewMenuButton(text string, width, height int) *Button {
	b := &Button{
		BaseWidget: NewBaseWidget(ui), enabled: true, text: text, menuStyle: true,
		buttonLayout: &ButtonLayout{HasImage: true, AllowFrameChange: true, DisabledColor: whiteAlpha100},
	}
	b.width, b.height = width, height
	l := ui.NewLabel(d2resource.Font16, d2resource.PaletteUnits)
	// Its ink is cached below; it never takes input and must not survive a
	// screen reset through the input manager's handler list.
	if err := ui.inputManager.UnbindHandler(l); err != nil {
		ui.Error(err.Error())
	}
	l.SetText(text)
	l.Color[0] = color.RGBA{R: 232, G: 221, B: 196, A: 255}
	l.Alignment = HorizontalAlignLeft
	lw, lh := l.GetSize()
	fit := math.Min(1, math.Min(float64(width-48)/float64(lw), float64(height-12)/float64(lh)))
	drawW, drawH := int(math.Ceil(float64(lw)*fit)), int(math.Ceil(float64(lh)*fit))
	x, y := (width-drawW)/2, (height-drawH)/2
	b.labelRect = image.Rect(x, y, x+drawW, y+drawH)
	l.SetPosition(0, 0)
	face := func(pressed bool, ink color.Color) d2interface.Surface {
		s := ui.renderer.NewSurface(width, height)
		s.DrawRect(width, height, color.RGBA{R: 17, G: 21, B: 20, A: 244})
		s.DrawRect(width, 1, color.RGBA{R: 104, G: 88, B: 58, A: 255})
		s.PushTranslation(0, height-1)
		s.DrawRect(width, 1, color.RGBA{R: 59, G: 54, B: 42, A: 255})
		s.Pop()
		l.Color[0] = ink
		// Scale the complete text image so advances and glyphs shrink together.
		// Surface.PushScale alone does not scale Label's translations.
		textSurface := ui.renderer.NewSurface(lw, lh)
		l.Render(textSurface)
		textY := y
		if pressed {
			textY++
		}
		s.PushTranslation(x, textY)
		s.PushScale(fit, fit)
		s.Render(textSurface)
		s.PopN(2)
		return s
	}
	b.normalSurface = face(false, l.Color[0])
	b.pressedSurface = face(true, color.RGBA{R: 241, G: 197, B: 120, A: 255})
	b.disabledSurface = face(false, color.RGBA{R: 107, G: 108, B: 99, A: 255})
	ui.addWidget(b)
	return b
}
