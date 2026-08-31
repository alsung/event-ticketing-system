'use client';

import { useEffect, useState, useTransition } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';

/** Search input for the events listing.
 *
 *  The query lives in the URL rather than in component state, so a search is
 *  shareable, survives a reload, and works with the back button. The page is a
 *  Server Component, so changing the URL re-runs the query on the server. */
export function SearchBox({ resultCount }: { resultCount: number }) {
    const router = useRouter();
    const pathname = usePathname();
    const params = useSearchParams();
    const [isPending, startTransition] = useTransition();

    const urlQuery = params.get('q') ?? '';
    const [value, setValue] = useState(urlQuery);

    // Debounced: typing should not fire a request per keystroke, but waiting
    // for a submit would make search feel like a form rather than a filter.
    useEffect(() => {
        if (value === urlQuery) return;

        const timer = setTimeout(() => {
            const next = new URLSearchParams(params.toString());
            if (value.trim()) {
                next.set('q', value.trim());
            } else {
                next.delete('q');
            }
            startTransition(() => {
                // replace, not push: every keystroke would otherwise become a
                // separate history entry to back out of.
                router.replace(`${pathname}?${next.toString()}`, { scroll: false });
            });
        }, 250);

        return () => clearTimeout(timer);
    }, [value, urlQuery, params, pathname, router]);

    return (
        <div className="flex flex-col gap-2">
            <label htmlFor="event-search" className="sr-only">Search events</label>

            <div className="relative">
                <svg
                    width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true"
                    className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-text-muted"
                >
                    <circle cx="11" cy="11" r="7" stroke="currentColor" strokeWidth="1.8" />
                    <path d="M20 20l-3.5-3.5" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
                </svg>

                <input
                    id="event-search"
                    type="search"
                    value={value}
                    onChange={(e) => setValue(e.target.value)}
                    placeholder="Search by artist, event or city"
                    className="h-11 w-full rounded-md border border-border-strong bg-surface pl-11 pr-3 text-[15px] text-text placeholder:text-text-muted transition-[border-color] duration-[120ms] focus:border-accent"
                />
            </div>

            {/* Announced politely so a screen reader hears the result count
                change without focus leaving the input. */}
            <p aria-live="polite" className="min-h-[20px] text-[13px] text-text-muted">
                {isPending
                    ? 'Searching…'
                    : urlQuery
                        ? `${resultCount} ${resultCount === 1 ? 'event' : 'events'} matching “${urlQuery}”`
                        : ''}
            </p>
        </div>
    );
}
