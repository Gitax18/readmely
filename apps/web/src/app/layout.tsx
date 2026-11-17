import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Readmely status",
  description: "Current status of the Readmely services.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-screen font-sans antialiased">{children}</body>
    </html>
  );
}
