class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.5.0/gopssh_Darwin_x86_64.tar.gz"
  version "0.5.0"
  sha256 "7f21a4b1c562a3823095e98639e0bd256c0ccf37f611e09f8a6285f3faaf0788"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
