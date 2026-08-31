'use client';

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
    variant?: 'primary' | 'secondary' | 'danger';
    /** Renders a spinner and disables the control. Async work needs a visible
     *  pending state, or the only feedback for a slow request is nothing at
     *  all, and the user presses again. */
    loading?: boolean;
    fullWidth?: boolean;
}

const variants = {
    primary: 'bg-accent text-on-accent hover:bg-accent-hover',
    secondary: 'bg-surface text-text border border-border-strong hover:bg-surface-hover',
    danger: 'bg-danger text-white hover:bg-danger-hover',
};

export function Button({
    variant = 'primary', loading, fullWidth, children, disabled, className = '', ...rest
}: ButtonProps) {
    return (
        <button
            {...rest}
            disabled={disabled || loading}
            aria-busy={loading || undefined}
            className={[
                'inline-flex h-11 items-center justify-center gap-2 rounded-md px-4',
                'text-[15px] font-medium',
                // Feedback on press, not on release. The scale is small enough
                // to read as a press rather than an animation.
                'transition-[background-color,transform,opacity] duration-[120ms]',
                'active:scale-[.98]',
                'disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100',
                variants[variant],
                fullWidth ? 'w-full' : '',
                className,
            ].join(' ')}
        >
            {loading && (
                <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" aria-hidden="true">
                    <circle cx="12" cy="12" r="9" stroke="currentColor" strokeWidth="2.5"
                        fill="none" opacity=".25" />
                    <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="2.5"
                        fill="none" strokeLinecap="round" />
                </svg>
            )}
            {children}
        </button>
    );
}
