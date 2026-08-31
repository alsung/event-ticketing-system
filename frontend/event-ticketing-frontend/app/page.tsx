import Link from 'next/link';

/** The landing page. It replaces the create-next-app template that shipped
 *  here, which was the first thing anyone opening the app would see. */
export default function HomePage() {
    return (
        <div className="flex flex-col gap-14 py-10">
            <section className="flex max-w-2xl flex-col gap-5">
                <span className="t-label text-accent">Event ticketing</span>

                <h1 className="text-balance">
                    Book tickets without the double charge.
                </h1>

                <p className="text-lg leading-relaxed text-text-secondary">
                    A ticketing platform built around one hard problem: selling a fixed pool
                    of seats to many simultaneous buyers without ever selling the same seat
                    twice, and taking payment exactly once even when the network disagrees.
                </p>

                <div className="flex flex-wrap items-center gap-3 pt-1">
                    <Link
                        href="/events"
                        className="inline-flex h-11 items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]"
                    >
                        Browse events
                    </Link>
                    <Link
                        href="/register"
                        className="inline-flex h-11 items-center rounded-md border border-border-strong bg-surface px-5 text-[15px] font-medium text-text transition-[background-color,transform] duration-[120ms] hover:bg-surface-hover active:scale-[.98]"
                    >
                        Create an account
                    </Link>
                </div>
            </section>

            {/* The engineering is the point of this project, so the landing page
                says what it does rather than showing generic marketing copy. */}
            <section aria-labelledby="how" className="flex flex-col gap-5">
                <h2 id="how" className="t-label text-text-muted">How it works</h2>

                <ul className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                    {[
                        {
                            title: 'No overselling',
                            body: 'Each purchase claims a seat under a row-level lock, so a hundred simultaneous buyers get a hundred different tickets — or an honest sold-out.',
                        },
                        {
                            title: 'Charged once',
                            body: 'Every purchase carries an idempotency key, forwarded to the payment processor, so a retried request returns the original charge rather than making a second one.',
                        },
                        {
                            title: 'Confirmed asynchronously',
                            body: 'Confirmations are published through a transactional outbox, so they survive a broker outage instead of vanishing with it.',
                        },
                    ].map((card) => (
                        <li
                            key={card.title}
                            className="flex flex-col gap-2 rounded-lg border border-border bg-surface p-5 shadow-e1"
                        >
                            <h3>{card.title}</h3>
                            <p className="text-[15px] text-text-secondary">{card.body}</p>
                        </li>
                    ))}
                </ul>
            </section>
        </div>
    );
}
