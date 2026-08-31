import './globals.css';
import type { Metadata } from "next";
import { Inter } from 'next/font/google';
import { UserProvider } from '@/context/UserContext';
import Navbar from '@/components/Navbar';

// Exposed as a CSS variable rather than a class, so globals.css can use it in
// the font stack and Tailwind can pick it up through the theme.
const inter = Inter({
  subsets: ["latin"],
  variable: '--font-inter',
  display: 'swap',
});

export const metadata: Metadata = {
  title: "EventMaster",
  description: "Browse events, buy tickets, and manage your bookings",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className={inter.variable}>
      <body>
        <UserProvider>
          <a href="#main" className="skip-link">Skip to content</a>
          <Navbar />
          <main id="main" className="mx-auto w-full max-w-5xl px-5 py-8">
            {children}
          </main>
        </UserProvider>
      </body>
    </html>
  );
}
