class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.5.2/gopssh_Darwin_x86_64.tar.gz"
  version "0.5.2"
  sha256 "59ccf11397d2eb4dbcddbd9118131c07d2c828e19e2c4bc28bf5e31542223caa"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
