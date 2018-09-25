class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.1.0/gopssh_Darwin_x86_64.tar.gz"
  version "0.1.0"
  sha256 "8fb4796286ef3e9d1ed72458b44c9a06723d44be8aa99da6ac6e75b3ed063805"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
