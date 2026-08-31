'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useUser } from '@/context/UserContext';

/** Inline rather than a dependency: two icons do not justify an icon package.
 *  An SVG also inherits currentColor and scales, which an emoji cannot. */
function TicketIcon() {
    return (
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <path
                d="M4 8.5A1.5 1.5 0 0 1 5.5 7h13A1.5 1.5 0 0 1 20 8.5v2a2 2 0 0 0 0 3.99v2A1.5 1.5 0 0 1 18.5 18h-13A1.5 1.5 0 0 1 4 16.5v-2a2 2 0 0 0 0-3.99v-2Z"
                stroke="currentColor" strokeWidth="1.6" strokeLinejoin="round"
            />
            <path d="M14 7v11" stroke="currentColor" strokeWidth="1.6" strokeDasharray="2 2.5" />
        </svg>
    );
}

export default function Navbar() {
    const { user, signOut } = useUser();
    const pathname = usePathname();

    // Current location has to be visible in the navigation, not merely implied
    // by the page content.
    const linkClass = (href: string) => {
        const active = pathname === href;
        return [
            'rounded-md px-3 py-2 text-sm font-medium transition-colors duration-[120ms]',
            active
                ? 'bg-accent-subtle text-accent'
                : 'text-text-secondary hover:bg-surface-hover hover:text-text',
        ].join(' ');
    };

    return (
        <header className="sticky top-0 z-20 border-b border-border bg-surface/80 backdrop-blur-md">
            <nav
                aria-label="Main"
                className="mx-auto flex h-14 w-full max-w-5xl items-center justify-between gap-4 px-5"
            >
                <Link
                    href={user ? '/events' : '/'}
                    className="flex items-center gap-2 rounded-md text-[15px] font-semibold tracking-[-.01em] text-text"
                >
                    <span className="text-accent"><TicketIcon /></span>
                    EventMaster
                </Link>

                <div className="flex items-center gap-1">
                    {user ? (
                        <>
                            <Link href="/events" className={linkClass('/events')}>Events</Link>
                            <Link href="/tickets" className={linkClass('/tickets')}>My tickets</Link>
                            <button
                                onClick={signOut}
                                className="ml-2 rounded-md px-3 py-2 text-sm font-medium text-text-secondary transition-colors duration-[120ms] hover:bg-danger-subtle hover:text-danger active:scale-[.97]"
                            >
                                Sign out
                            </button>
                        </>
                    ) : (
                        <>
                            <Link href="/login" className={linkClass('/login')}>Log in</Link>
                            <Link
                                href="/register"
                                className="ml-1 rounded-md bg-accent px-3.5 py-2 text-sm font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.97]"
                            >
                                Sign up
                            </Link>
                        </>
                    )}
                </div>
            </nav>
        </header>
    );
}
