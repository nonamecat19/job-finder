import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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

    expect(await screen.findByText('Generation failed')).toBeInTheDocument();
    expect(screen.getByText('resume generation failed')).toBeInTheDocument();
  });

  it('shows toasts emitted from outside React via the bus', async () => {
    render(
      <ToastProvider>
        <div />
      </ToastProvider>,
    );

    emitToast({ title: 'Background job done', variant: 'info' });

    expect(await screen.findByText('Background job done')).toBeInTheDocument();
  });

  it('dismisses a toast when the close button is clicked', async () => {
    const user = userEvent.setup();
    render(
      <ToastProvider>
        <Harness />
      </ToastProvider>,
    );

    await user.click(screen.getByText('raise-success'));
    expect(await screen.findByText('Saved')).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Dismiss'));
    await waitFor(() => expect(screen.queryByText('Saved')).not.toBeInTheDocument());
  });
});
