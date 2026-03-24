import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import { Toaster } from "sonner";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "VoidDB — Admin Panel",
  description:
    "High-performance document database with S3-compatible blob storage",
  icons: { icon: "/favicon.ico" },
};

/**
 * Root layout wraps every page with fonts, the Toaster notification provider,
 * and global styles.
 */
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
              background: "rgba(10,10,31,0.9)",
              border: "1px solid rgba(96,96,255,0.3)",
              color: "#e0e0ff",
            },
          }}
        />
      </body>
    </html>
  );
}
