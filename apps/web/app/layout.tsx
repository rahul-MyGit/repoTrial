import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "RepoTrial — practice debugging with AI",
  description:
    "Read a failing run, diagnose an unfamiliar repository, and guide AI through a careful fix.",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
