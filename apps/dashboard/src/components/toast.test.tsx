import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ToastProvider, useToast } from './toast';
import { emitToast } from '../lib/toastBus';

function Harness() {
  const { error, success } = useToast();
  return (
    <div>
      <button onClick={() => error('Generation failed', new Error('resume generation failed'))}>raise-error</button>
      <button onClick={() => success('Saved', 'All good')}>raise-success</button>
    </div>
  );
}

describe('ToastProvider', () => {
  it('renders an error toast with title and description', async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );

    await user.click(screen.getByText('raise-error'));

    await waitFor(() => {
      expect(screen.getByText('Generation failed')).toBeInTheDocument();
    });
    expect(screen.getByText('resume generation failed')).toBeInTheDocument();
  });

  it('shows toasts emitted from outside React via the bus', async () => {
    render(
      <ToastProvider>
        <div />
      </ToastProvider>,
    );

    emitToast({ title: 'Background job done', variant: 'info' });

    await waitFor(() => {
      expect(screen.getByText('Background job done')).toBeInTheDocument();
    });
  });

  it('dismisses a toast when the close button is clicked', async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );

    await user.click(screen.getByText('raise-success'));

    await waitFor(() => {
      expect(screen.getByText('Saved')).toBeInTheDocument();
    });

    const closeButtons = screen.getAllByLabelText('Close');
    await user.click(closeButtons[0]);

    await waitFor(() => {
      expect(screen.queryByText('Saved')).not.toBeInTheDocument();
    });
  });
});
