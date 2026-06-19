# Homebrew formula for Peapod.
#
# Install the latest from main (once this repo is on GitHub — replace YOURUSER):
#   brew install --HEAD YOURUSER/tap/peapod
#
# For a tagged release, fill in `url` + `sha256` below and use `brew install`.
class Peapod < Formula
  desc "Disposable, isolated sandboxes for AI agents"
  homepage "https://github.com/YOURUSER/peapod"
  license "MIT"
  head "https://github.com/YOURUSER/peapod.git", branch: "main"

  # url "https://github.com/YOURUSER/peapod/archive/refs/tags/v0.1.0.tar.gz"
  # sha256 "REPLACE_WITH_RELEASE_TARBALL_SHA256"

  depends_on "go" => :build

  def install
    system "go", "build", "-o", bin/"peapod", "./cmd/peapod"
  end

  test do
    assert_match "peapod 0.1.0", shell_output("#{bin}/peapod version")
  end
end
