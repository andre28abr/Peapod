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
  url "https://github.com/andre28abr/Peapod/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "a719d04b660575c5226c8426d187633b34ecdaf1ade8ec80169b508204650fa8"
  license "AGPL-3.0-only"
  head "https://github.com/andre28abr/Peapod.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", "-o", bin/"peapod", "./cmd/peapod"
    # Static linux binary that runs the egress proxy inside the firewall sidecar
    # (used by the bypass-proof --allow firewall).
    arch = Hardware::CPU.arm? ? "arm64" : "amd64"
    system "env", "GOOS=linux", "GOARCH=#{arch}", "CGO_ENABLED=0",
           "go", "build", "-o", bin/"peapod-linux-#{arch}", "./cmd/peapod"
  end

  test do
    assert_match "peapod 0.1.0", shell_output("#{bin}/peapod version")
  end
end
