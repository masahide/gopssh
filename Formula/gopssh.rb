class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.4.0/gopssh_Darwin_x86_64.tar.gz"
  version "0.4.0"
  sha256 "a86f4d76e84aa3858283611dea3c6679e5b2ed30bc9e7fcdef7e56c7e2f72f05"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
