class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.2.0/gopssh_Darwin_x86_64.tar.gz"
  version "0.2.0"
  sha256 "538641be2e6d1ab9c6312e02a9bd9e2c8e8fd1d1f58b1cb78b25a75167088e5e"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
