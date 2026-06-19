// Renders the Peapod app icon (a pea pod) into a .iconset folder using
// Core Graphics — no external image tools needed. Usage: Icon <iconset-dir>
import AppKit

let specs: [(String, Int)] = [
    ("icon_16x16", 16), ("icon_16x16@2x", 32),
    ("icon_32x32", 32), ("icon_32x32@2x", 64),
    ("icon_128x128", 128), ("icon_128x128@2x", 256),
    ("icon_256x256", 256), ("icon_256x256@2x", 512),
    ("icon_512x512", 512), ("icon_512x512@2x", 1024),
]

func col(_ r: CGFloat, _ g: CGFloat, _ b: CGFloat, _ a: CGFloat = 1) -> CGColor {
    CGColor(srgbRed: r / 255, green: g / 255, blue: b / 255, alpha: a)
}

func render(_ size: Int) -> Data {
    let s = CGFloat(size)
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: size, pixelsHigh: size,
                               bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
                               isPlanar: false, colorSpaceName: .deviceRGB,
                               bytesPerRow: 0, bitsPerPixel: 0)!
    let gctx = NSGraphicsContext(bitmapImageRep: rep)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = gctx
    let cg = gctx.cgContext
    cg.scaleBy(x: s / 512, y: s / 512) // draw in a fixed 512 coordinate space

    // background squircle with a green gradient
    let bg = CGRect(x: 44, y: 44, width: 424, height: 424)
    let bgPath = CGPath(roundedRect: bg, cornerWidth: 96, cornerHeight: 96, transform: nil)
    cg.saveGState()
    cg.addPath(bgPath)
    cg.clip()
    let grad = CGGradient(colorsSpace: CGColorSpaceCreateDeviceRGB(),
                          colors: [col(116, 201, 71), col(60, 138, 44)] as CFArray,
                          locations: [0, 1])!
    cg.drawLinearGradient(grad, start: CGPoint(x: 256, y: 468), end: CGPoint(x: 256, y: 44), options: [])
    cg.setFillColor(col(255, 255, 255, 0.10))
    cg.fillEllipse(in: CGRect(x: 76, y: 300, width: 360, height: 172)) // gloss
    cg.restoreGState()

    // pea pod + peas, tilted
    cg.saveGState()
    cg.translateBy(x: 256, y: 256)
    cg.rotate(by: -28 * .pi / 180)
    cg.translateBy(x: -256, y: -256)
    let pod = CGMutablePath()
    pod.move(to: CGPoint(x: 100, y: 256))
    pod.addCurve(to: CGPoint(x: 412, y: 256), control1: CGPoint(x: 160, y: 350), control2: CGPoint(x: 352, y: 350))
    pod.addCurve(to: CGPoint(x: 100, y: 256), control1: CGPoint(x: 352, y: 162), control2: CGPoint(x: 160, y: 162))
    pod.closeSubpath()
    cg.addPath(pod); cg.setFillColor(col(166, 225, 94)); cg.fillPath()
    cg.addPath(pod); cg.setStrokeColor(col(47, 122, 38)); cg.setLineWidth(10); cg.setLineJoin(.round); cg.strokePath()
    for cx in [190.0, 256.0, 322.0] {
        let r: CGFloat = 42
        let pe = CGRect(x: CGFloat(cx) - r, y: 256 - r, width: 2 * r, height: 2 * r)
        cg.addEllipse(in: pe); cg.setFillColor(col(215, 243, 154)); cg.fillPath()
        cg.addEllipse(in: pe); cg.setStrokeColor(col(79, 158, 46)); cg.setLineWidth(5); cg.strokePath()
    }
    cg.restoreGState()

    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

let outDir = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "Peapod.iconset"
try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)
for (name, px) in specs {
    try! render(px).write(to: URL(fileURLWithPath: "\(outDir)/\(name).png"))
}
try! render(256).write(to: URL(fileURLWithPath: "\(outDir)/../peapod-icon-preview.png"))
print("wrote iconset to \(outDir)")
