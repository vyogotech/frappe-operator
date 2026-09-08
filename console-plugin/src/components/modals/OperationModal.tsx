import * as React from 'react';
import {
  Alert,
  Button,
  Form,
  Modal,
  ModalBody,
  ModalFooter,
  ModalHeader,
  ModalVariant,
  Spinner,
} from '@patternfly/react-core';
import { useThemeClass } from '../PluginRoot';

export interface OperationModalProps {
  title: string;
  /** Short explanation of what the operator will do once this is submitted. */
  description?: React.ReactNode;
  submitLabel: string;
  submitVariant?: 'primary' | 'danger';
  isSubmitDisabled?: boolean;
  variant?: ModalVariant;
  /** Rejecting leaves the modal open and surfaces the error inline. */
  onSubmit: () => Promise<void>;
  closeModal: () => void;
  children: React.ReactNode;
}

/**
 * Shared chrome for the site and bench operation modals: submit-in-progress
 * state, inline error reporting, and consistent footer buttons. The modal stays
 * open on failure so the operator's message sits next to the inputs that caused
 * it.
 */
export const OperationModal: React.FC<OperationModalProps> = ({
  title,
  description,
  submitLabel,
  submitVariant = 'primary',
  isSubmitDisabled,
  variant = ModalVariant.medium,
  onSubmit,
  closeModal,
  children,
}) => {
  // PatternFly portals the modal to document.body, outside the plugin root, so
  // any plugin-scoped styling has to travel with it.
  const themeClass = useThemeClass();
  const [inProgress, setInProgress] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>();

  const handleSubmit = React.useCallback(async () => {
    setInProgress(true);
    setError(undefined);
    try {
      await onSubmit();
      closeModal();
    } catch (e) {
      const message =
        (e as { message?: string })?.message ?? 'The request was rejected by the API server.';
      setError(message);
      setInProgress(false);
    }
  }, [onSubmit, closeModal]);

  return (
    <Modal className={themeClass} variant={variant} isOpen onClose={closeModal}>
      <ModalHeader title={title} description={description} />

      <ModalBody>
        <Form
          onSubmit={(e) => {
            e.preventDefault();
          }}
        >
          {error && (
            <Alert variant="danger" isInline title="Could not start the operation">
              {error}
            </Alert>
          )}
          {children}
        </Form>
      </ModalBody>

      <ModalFooter>
        <Button
          variant={submitVariant}
          onClick={handleSubmit}
          isDisabled={isSubmitDisabled || inProgress}
          icon={inProgress ? <Spinner size="sm" /> : undefined}
        >
          {submitLabel}
        </Button>
        <Button variant="link" onClick={closeModal} isDisabled={inProgress}>
          Cancel
        </Button>
      </ModalFooter>
    </Modal>
  );
};

export default OperationModal;
