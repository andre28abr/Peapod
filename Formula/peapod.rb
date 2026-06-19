# Homebrew formula for Peapod.
#
# Install from a tap once published (e.g. andre28abr/homebrew-tap):
#   brew install --HEAD andre28abr/tap/peapod
# Or build straight from this repo's formula:
#   brew install --HEAD ./Formula/peapod.rb
#
# For a tagged release, fill in `url` + `sha256` and use `brew install`.
class Peapod < Formula
  desc "Disposable, isolated sandboxes for AI agents"
  homepage "https://github.com/andre28abr/Peapod"
  license "AGPL-3.0-only"
  head "https://github.com/andre28abr/Peapod.git", branch: "main"

  # url "https://github.com/andre28abr/Peapod/archive/refs/tags/v0.1.0.tar.gz"
  # sha256 "REPLACE_WITH_RELEASE_TARBALL_SHA256"

  depends_on "go" => :build

  def install
    system "go", "build", "-o", bin/"peapod", "./cmd/peapod"
  end

  test do
    assert_match "peapod 0.1.0", shell_output("#{bin}/peapod version")
  end
end
