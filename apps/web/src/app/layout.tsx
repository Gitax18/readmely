import type { Metadata, Viewport } from "next";
import { DM_Mono, Manrope } from "next/font/google";
import { headers } from "next/headers";
import { MotionProvider } from "@/components/ui/motion";
import { THEME_KEY } from "@/components/ui/theme-toggle";
import "./globals.css";

// Both fonts are downloaded at build time and served from this site, which the content security
// policy requires (font-src 'self').
const sans = Manrope({ subsets: ["latin"], variable: "--font-manrope", display: "swap" });
const mono = DM_Mono({
  subsets: ["latin"],
  weight: ["400", "500"],
  variable: "--font-dm-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Readmely: a README, a context pack and a diagram from any repository",
  description:
    "Paste a public GitHub repository. Readmely reads the code, then writes a README, a context pack for AI coding tools and an architecture diagram.",
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#fbfcfd" },
    { media: "(prefers-color-scheme: dark)", color: "#101719" },
  ],
};

// Runs before first paint so a dark visitor never sees a white flash. A saved choice wins;
// otherwise the system setting decides.
const themeScript = `(function(){try{var s=localStorage.getItem(${JSON.stringify(THEME_KEY)});var d=s?s==="dark":matchMedia("(prefers-color-scheme: dark)").matches;document.documentElement.classList.toggle("dark",d)}catch(e){}})()`;

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  // Reading request headers makes every page render per request, which the
  // per-request CSP nonce requires. Static pages would carry no valid nonce.
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`} suppressHydrationWarning>
      <head>
        <script
          nonce={nonce}
          suppressHydrationWarning
          dangerouslySetInnerHTML={{ __html: themeScript }}
        />
      </head>
      <body className="min-h-screen font-sans antialiased">
        <MotionProvider nonce={nonce}>{children}</MotionProvider>
      </body>
    </html>
  );
}
