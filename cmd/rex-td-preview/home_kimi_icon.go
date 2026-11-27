package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

func homeRexKimiIcon(ctx context.Context, cache string) []byte {
	const bundle = "/Applications/Rex.app/Contents/Resources/SuperBlockIcons_SuperBlockIcons.bundle"
	info, err := os.Stat(filepath.Join(bundle, "Contents", "Resources", "Assets.car"))
	if err != nil {
		return nil
	}
	return cachedHomeIcon(cache, "rex-kimi.png", info.ModTime(), func(output string) bool {
		return exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", rexKimiGlyphPNG, bundle, output).Run() == nil
	})
}

// Rex supplies a template glyph. Apply the home title color and leave the
// background transparent so the mark follows the row's selection and stripe.
const rexKimiGlyphPNG = `
ObjC.import("AppKit");
function run(args) {
  const bundle = $.NSBundle.bundleWithPath(args[0]);
  const glyph = bundle.imageForResource("block-glyph-kimi");
  if (glyph.isNil()) throw new Error("Rex Kimi glyph unavailable");
  const size = glyph.size;
  const scale = 58 / Math.max(size.width, size.height);
  const width = size.width * scale;
  const height = size.height * scale;
  const bounds = $.NSMakeRect(0, 0, 64, 64);
  const bitmap = $.NSBitmapImageRep.alloc
    .initWithBitmapDataPlanesPixelsWidePixelsHighBitsPerSampleSamplesPerPixelHasAlphaIsPlanarColorSpaceNameBytesPerRowBitsPerPixel(
      null, 64, 64, 8, 4, true, false, $.NSDeviceRGBColorSpace, 0, 0);
  const graphics = $.NSGraphicsContext.graphicsContextWithBitmapImageRep(bitmap);
  $.NSGraphicsContext.saveGraphicsState;
  $.NSGraphicsContext.setCurrentContext(graphics);
  $.NSColor.clearColor.setFill;
  $.NSRectFillUsingOperation(bounds, $.NSCompositingOperationCopy);
  glyph.drawInRectFromRectOperationFraction(
    $.NSMakeRect((64-width)/2, (64-height)/2, width, height),
    $.NSZeroRect, $.NSCompositingOperationSourceOver, 1);
  $.NSColor.colorWithSRGBRedGreenBlueAlpha(240/255, 243/255, 246/255, 1).setFill;
  $.NSRectFillUsingOperation(bounds, $.NSCompositingOperationSourceIn);
  $.NSGraphicsContext.restoreGraphicsState;
  const png = bitmap.representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $({}));
  if (png.isNil() || !png.writeToFileAtomically(args[1], true)) {
    throw new Error("Could not write Rex Kimi glyph");
  }
}
`
