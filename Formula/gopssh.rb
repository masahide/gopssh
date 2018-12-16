class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.5.1/gopssh_Darwin_x86_64.tar.gz"
  version "0.5.1"
  sha256 "6155305c654f8fde369b0540dbfd05503b3e3133832a75010db0e2198f0cec23"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
