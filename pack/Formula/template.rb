# Documentation: https://docs.brew.sh/Formula-Cookbook
#                https://rubydoc.brew.sh/Formula
# PLEASE REMOVE ALL GENERATED COMMENTS BEFORE SUBMITTING YOUR PULL REQUEST!
class Gopssh < Formula
  desc "AWS assume role credential wrapper"
  homepage "https://github.com/masahide/gopssh"
  license "MIT"
  version "1.0.0"
  on_macos do
    on_arm do
      url "https://github.com/masahide/gopssh/releases/download/__version__/darwin-arm64.tar.gz"
      sha256 "__darwin-arm64_sha256__"
    end
    on_intel do
      url "https://github.com/masahide/gopssh/releases/download/__version__/darwin-amd64.tar.gz"
      sha256 "__darwin-amd64_sha256__"
    end
  end
  on_linux do
    on_arm do
      url "https://github.com/masahide/gopssh/releases/download/__version__/linux-arm64.tar.gz"
      sha256 "__linux-arm64_sha256__"
    end
    on_intel do
      url "https://github.com/masahide/gopssh/releases/download/__version__/linux-amd64.tar.gz"
	  sha256 "__linux-amd64_sha256__"
    end
  end

  # depends_on "cmake" => :build

  def install
    system "chmod", "755", "gopssh"
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
