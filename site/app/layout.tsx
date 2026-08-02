import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Muamba · Trust remote files once",
  description: "TOFU vendoring and integrity verification for remote build assets.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
