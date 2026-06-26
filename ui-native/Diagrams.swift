// Renders the small explanatory diagrams (PNG) used in the "Para todos" guide.
// Usage: Diagrams <output-dir>   (defaults to ../docs)
import AppKit

func col(_ r: CGFloat, _ g: CGFloat, _ b: CGFloat, _ a: CGFloat = 1) -> NSColor {
    NSColor(srgbRed: r / 255, green: g / 255, blue: b / 255, alpha: a)
}

func text(_ s: String, _ x: CGFloat, _ y: CGFloat, size: CGFloat, color: NSColor, bold: Bool = false) {
    let f = bold ? NSFont.boldSystemFont(ofSize: size) : NSFont.systemFont(ofSize: size)
    (s as NSString).draw(at: CGPoint(x: x, y: y), withAttributes: [.font: f, .foregroundColor: color])
}

func roundRect(_ cg: CGContext, _ r: CGRect, _ rad: CGFloat, fill: NSColor, stroke: NSColor?, dashed: Bool = false, lw: CGFloat = 2) {
    let p = CGPath(roundedRect: r, cornerWidth: rad, cornerHeight: rad, transform: nil)
    cg.addPath(p); cg.setFillColor(fill.cgColor); cg.fillPath()
    if let s = stroke {
        cg.addPath(p); cg.setStrokeColor(s.cgColor); cg.setLineWidth(lw)
        if dashed { cg.setLineDash(phase: 0, lengths: [6, 4]) }
        cg.strokePath()
        cg.setLineDash(phase: 0, lengths: [])
    }
}

func arrow(_ cg: CGContext, fromX: CGFloat, toX: CGFloat, y: CGFloat, color: NSColor) {
    cg.setStrokeColor(color.cgColor); cg.setLineWidth(3); cg.setLineCap(.round); cg.setLineJoin(.round)
    cg.move(to: CGPoint(x: fromX, y: y)); cg.addLine(to: CGPoint(x: toX, y: y)); cg.strokePath()
    cg.move(to: CGPoint(x: toX - 9, y: y + 7)); cg.addLine(to: CGPoint(x: toX, y: y)); cg.addLine(to: CGPoint(x: toX - 9, y: y - 7)); cg.strokePath()
}

func render(_ w: Int, _ h: Int, _ draw: (CGContext) -> Void) -> Data {
    let s = 2
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: w * s, pixelsHigh: h * s,
                               bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true,
                               isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    let gctx = NSGraphicsContext(bitmapImageRep: rep)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = gctx
    gctx.cgContext.scaleBy(x: CGFloat(s), y: CGFloat(s))
    draw(gctx.cgContext)
    NSGraphicsContext.restoreGraphicsState()
    return rep.representation(using: .png, properties: [:])!
}

let outDir = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "../docs"

// 1) A metáfora da caixa: o sandbox isolado dentro do seu Mac.
let caixa = render(820, 280) { cg in
    roundRect(cg, CGRect(x: 30, y: 30, width: 760, height: 220), 16,
              fill: col(241, 239, 232, 0.55), stroke: col(180, 178, 169), lw: 2)
    text("Seu Mac", 52, 222, size: 17, color: col(44, 44, 42), bold: true)
    text("fica intacto — seus arquivos, sua rede, suas senhas", 52, 198, size: 12.5, color: col(95, 94, 90))
    roundRect(cg, CGRect(x: 430, y: 60, width: 330, height: 152), 14,
              fill: col(225, 245, 238), stroke: col(29, 158, 117), dashed: true, lw: 2)
    text("Sandbox", 452, 182, size: 15, color: col(8, 80, 65), bold: true)
    text("o código (ou a IA) roda aqui", 452, 156, size: 12.5, color: col(8, 80, 65))
    text("isolado · sem internet", 452, 132, size: 12, color: col(15, 110, 86))
    text("descartável — é só jogar fora", 452, 110, size: 12, color: col(15, 110, 86))
}
try! caixa.write(to: URL(fileURLWithPath: "\(outDir)/guia-caixa.png"))

// 2) O fluxo em três passos.
let fluxo = render(860, 190) { cg in
    roundRect(cg, CGRect(x: 20, y: 40, width: 240, height: 110), 14,
              fill: col(250, 236, 231), stroke: col(216, 90, 48), lw: 1.5)
    text("1 · Criar", 44, 116, size: 15, color: col(74, 27, 12), bold: true)
    text("clique num modelo", 44, 90, size: 12.5, color: col(113, 43, 29))
    arrow(cg, fromX: 268, toX: 302, y: 95, color: col(150, 150, 150))
    roundRect(cg, CGRect(x: 310, y: 40, width: 240, height: 110), 14,
              fill: col(225, 245, 238), stroke: col(29, 158, 117), lw: 1.5)
    text("2 · Usar", 334, 116, size: 15, color: col(8, 80, 65), bold: true)
    text("rode comandos isolado", 334, 90, size: 12.5, color: col(15, 110, 86))
    arrow(cg, fromX: 558, toX: 592, y: 95, color: col(150, 150, 150))
    roundRect(cg, CGRect(x: 600, y: 40, width: 240, height: 110), 14,
              fill: col(241, 239, 232), stroke: col(136, 135, 128), lw: 1.5)
    text("3 · Descartar", 624, 116, size: 15, color: col(44, 44, 42), bold: true)
    text("jogue a caixa fora", 624, 90, size: 12.5, color: col(95, 94, 90))
}
try! fluxo.write(to: URL(fileURLWithPath: "\(outDir)/guia-fluxo.png"))

print("wrote diagrams to \(outDir)")
