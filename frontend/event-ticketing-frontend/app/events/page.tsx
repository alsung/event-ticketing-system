import Link from 'next/link';

/** A Server Component: browsing events needs no token and no interactivity, so
 *  the fetch happens on the server. That removes the loading effect entirely --
 *  Next streams loading.tsx while this awaits, and routes a failure to
 *  error.tsx, so neither state needs client state of its own. */

const GATEWAY = process.env.GATEWAY_URL ?? 'http://localhost:8000';

interface EventItem {
    id: string;
    name: string;
    description: string;
    location: string;
    start_time: string;
    end_time: string;
}

function formatRange(start: string, end: string) {
    const s = new Date(start);
    const e = new Date(end);
    return {
        date: s.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' }),
        time: `${s.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })} – ${e.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}`,
    };
}

async function getEvents(): Promise<EventItem[]> {
    // Inventory changes on every purchase, so a cached list would show seats
    // that are already gone.
    const res = await fetch(`${GATEWAY}/events`, { cache: 'no-store' });
    if (!res.ok) throw new Error(`events request failed: ${res.status}`);
    const data = await res.json();
    return Array.isArray(data) ? data : [];
}

export default async function EventsPage() {
    const events = await getEvents();

    return (
        <div className="flex flex-col gap-6">
            <div className="flex flex-col gap-1.5">
                <h1>Upcoming events</h1>
                <p className="text-[15px] text-text-secondary">
                    Browse what is on and book a ticket.
                </p>
            </div>

            {events.length === 0 ? (
                <div className="rounded-lg border border-dashed border-border-strong bg-surface-sunken px-6 py-12 text-center">
                    <h2 className="mb-1.5">No events yet</h2>
                    <p className="mx-auto max-w-sm text-[15px] text-text-secondary">
                        There is nothing scheduled right now. Check back soon, or seed the
                        development database with{' '}
                        <code className="rounded bg-surface px-1.5 py-0.5 text-[13px]">make seed</code>.
                    </p>
                </div>
            ) : (
                <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                    {events.map((event) => {
                        const { date, time } = formatRange(event.start_time, event.end_time);
                        return (
                            <li key={event.id}>
                                <Link
                                    href={`/events/${event.id}`}
                                    className="group flex h-full flex-col gap-2.5 rounded-lg border border-border bg-surface p-5 shadow-e1 transition-[border-color,box-shadow,transform] duration-[220ms] hover:border-border-strong hover:shadow-e2 active:scale-[.99]"
                                >
                                    <span className="t-label tnum text-accent">{date}</span>
                                    <h2 className="text-text group-hover:text-accent">{event.name}</h2>
                                    {event.description && (
                                        <p className="line-clamp-2 text-[15px] text-text-secondary">
                                            {event.description}
                                        </p>
                                    )}
                                    <div className="mt-auto flex flex-col gap-1 pt-2 text-[13px] text-text-muted">
                                        <span className="tnum">{time}</span>
                                        {event.location && <span>{event.location}</span>}
                                    </div>
                                </Link>
                            </li>
                        );
                    })}
                </ul>
            )}
        </div>
    );
}
