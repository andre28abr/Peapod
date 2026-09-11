# Homebrew formula for Peapod (installs the `peapod` CLI + MCP server).
#
# Install the tagged release straight from this formula:
#   brew install https://raw.githubusercontent.com/andre28abr/Peapod/main/Formula/peapod.rb
#
# Or the latest dev build from main:
#   brew install --HEAD https://raw.githubusercontent.com/andre28abr/Peapod/main/Formula/peapod.rb
class Peapod < Formula
  desc "Disposable, isolated sandboxes for AI agents"
  homepage "https://github.com/andre28abr/Peapod"
  url "https://github.com/andre28abr/Peapod/archive/refs/tags/v0.3.0.tar.gz"
  sha256 "17b6e137c64a130293109ffc42b1c5fb88b6fc063e9f7fed9e9b14bf5ba8f5e8"
  license "AGPL-3.0-only"
  head "https://github.com/andre28abr/Peapod.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", "-o", bin/"peapod", "./cmd/peapod"
    # Static linux binary that runs the egress proxy inside the firewall sidecar
    # (used by the bypass-proof --allow firewall).
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    with_env(GOOS: "linux", GOARCH: arch, CGO_ENABLED: "0") do
      system "go", "build", "-o", bin/"peapod-linux-#{arch}", "./cmd/peapod"
    end
  end

  test do
    assert_match "peapod 0.3.0", shell_output("#{bin}/peapod version")
  end
end
