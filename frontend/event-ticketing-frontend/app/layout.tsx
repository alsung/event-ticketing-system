import './globals.css';
import type { Metadata } from "next";
import { Inter } from 'next/font/google';
import { UserProvider } from '@/context/UserContext';
import Navbar from '@/components/Navbar';

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "EventMaster",
  description: "Browse and manage your events and tickets",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <UserProvider>
          <Navbar />
          <main className="p-4">{children}</main>
        </UserProvider>
      </body>
    </html>
  );
}
