'use client';

import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { useUser } from '@/context/UserContext';

export default function Navbar() {
    const { user, signOut } = useUser();

    return (
        <nav className="bg-white shadow-md px-6 py-4 flex justify-between items-center">
            <div className="text-xl font-semibold text-gray-800">
                <Link href={user ? "/events" : "/login"}>🎟️ EventMaster</Link>
            </div>

            <div className="flex items-center gap-4">
                {user ? (
                    <>
                        <Link href="/events" className="text-gray-700 hover:text-blue-600 transition">
                            My Events
                        </Link>
                        <Link href="/browse-events" className="text-gray-700 hover:text-blue-600 transition">
                            Browse Events
                        </Link>
                        <button
                            onClick={signOut}
                            className="bg-red-500 text-white px-3 py-1 rounded hover:bg-red-600 transition"
                        >
                            Sign Out
                        </button>
                    </>
                ) : (
                    <>
                        <Link href="/login" className="text-gray-700 hover:text-blue-600 transition">
                            Login
                        </Link>
                        <Link href="/register" className="text-gray-700 hover:text-blue-600 transition">
                            Register
                        </Link>
                    </>
                )}
            </div>
        </nav>
    );
}