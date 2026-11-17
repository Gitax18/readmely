import type { Metadata } from "next";
import { headers } from "next/headers";
import "./globals.css";

export const metadata: Metadata = {
  title: "Readmely status",
  description: "Current status of the Readmely services.",
};

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  // Reading request headers makes every page render per request, which the
  // per-request CSP nonce requires. Static pages would carry no valid nonce.
  await headers();
  return (
    <html lang="en">
      <body className="min-h-screen font-sans antialiased">{children}</body>
    </html>
  );
}
