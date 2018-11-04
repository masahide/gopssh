class Gopssh < Formula
  desc "parallel ssh client"
  homepage "https://github.com/masahide/gopssh"
  url "https://github.com/masahide/gopssh/releases/download/v0.3.0/gopssh_Darwin_x86_64.tar.gz"
  version "0.3.0"
  sha256 "9cc7b442406bc35e9153d32db73cf798346b304cc81541eecb7bf94c034a633a"

  def install
    bin.install "gopssh"
  end

  test do
    system "#{bin}/gopssh -v"
  end
end
