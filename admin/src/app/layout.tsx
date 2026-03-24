import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { Toaster } from "sonner";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "VoidDB — Console",
  description:
    "High-performance document database with S3-compatible blob storage",
  icons: { icon: "/favicon.ico" },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className="dark">
      <body className={`${inter.className} antialiased`}>
        {children}
        <Toaster
          theme="dark"
          position="bottom-right"
          toastOptions={{
            style: {
              background: "hsl(240 6% 9%)",
              border: "1px solid hsl(240 5% 18%)",
              color: "hsl(210 6% 93%)",
            },
          }}
        />
      </body>
    </html>
  );
}
