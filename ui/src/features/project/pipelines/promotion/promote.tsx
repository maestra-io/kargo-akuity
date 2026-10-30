import { faTruckArrowRight } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { Alert, Button, Checkbox, Drawer, Flex, Tooltip } from 'antd';
import classNames from 'classnames';
import { useMemo, useState } from 'react';
import { generatePath, useNavigate } from 'react-router-dom';

import { paths } from '@ui/config/paths';
import { useExtensionsContext } from '@ui/extensions/extensions-context';
import { ModalComponentProps } from '@ui/features/common/modal/modal-context';
import { getCurrentFreightForComparison } from '@ui/features/common/utils';
import { IAction, useActionContext } from '@ui/features/project/pipelines/context/action-context';
import { usePromoteDownstream, usePromoteToStage } from '@ui/gen/api/v2/core/core';
import { Freight, Stage } from '@ui/gen/api/v2/models';

import { useDictionaryContext } from '../context/dictionary-context';
import { isStageControlFlow, isStageTargetAware } from '../nodes/stage-meta-utils';

import { findDowngrades } from './downgrade';
import { FreightDetails } from './freight-details';
import styles from './promote.module.less';
import { isPromotionWindowClosed, promotionWindowClosedMessage } from './promotion-window';

type PromoteProps = ModalComponentProps & {
  stage: Stage;
  freight: Freight;
};

export const Promote = (props: PromoteProps) => {
  const actionContext = useActionContext();
  const navigate = useNavigate();
  const { promoteTabs } = useExtensionsContext();

  const dictionaryContext = useDictionaryContext();

  const isDownstreamPromotion =
    actionContext?.action?.type === IAction.PROMOTE_DOWNSTREAM || isStageControlFlow(props.stage);

  const freightAlias = props.freight?.alias;
  const stageName = props.stage?.metadata?.name;
  const projectName = props.stage?.metadata?.namespace;

  const currentFreightOnStage = useMemo(
    () => getCurrentFreightForComparison(props.stage, props.freight),
    [props.stage, props.freight]
  );

  // Kargo denies a promotion that would lower an artifact's version unless it
  // is marked as a rollback. Downstream promotions span several Stages, so the
  // drawer cannot pre-check them; the rollback box is still offered there and
  // the server reports any downgrade it denies.
  const downgrades = useMemo(
    () => (isDownstreamPromotion ? [] : findDowngrades(currentFreightOnStage, props.freight)),
    [isDownstreamPromotion, currentFreightOnStage, props.freight]
  );
  const [rollback, setRollback] = useState(false);

  const promoteActionMutation = usePromoteToStage({
    mutation: {
      onSuccess: (response) => {
        // A target-aware Stage promotes through a PromotionRequest, so the
        // response is a PromotionRequest and there is no Promotion to open.
        // Send the user to the Stage's Promotions tab, where the request and
        // whatever it reports are listed.
        if (isStageTargetAware(props.stage)) {
          navigate(
            generatePath(paths.stage, {
              name: projectName,
              stageName
            })
          );

          actionContext?.cancel();
          return;
        }

        // navigate
        navigate(
          generatePath(paths.promotion, {
            name: projectName,
            promotionId: response.data?.metadata?.name
          })
        );

        actionContext?.cancel();
      }
    }
  });

  const promoteDownstreamActionMutation = usePromoteDownstream({
    mutation: {
      onSuccess: () => {
        // navigate
        navigate(
          generatePath(paths.project, {
            name: projectName
          })
        );

        actionContext?.cancel();
      }
    }
  });

  const onPromote = () => {
    const payload = {
      stage: stageName || '',
      project: projectName || '',
      data: { freight: props.freight?.metadata?.name, rollback: rollback || undefined }
    };

    if (isDownstreamPromotion) {
      promoteDownstreamActionMutation.mutate(payload);
      return;
    }

    promoteActionMutation.mutate(payload);
  };

  // a closed promotion window forbids promotion of this Stage. Downstream
  // promotions target other Stages, each gated by its own window.
  const windowClosed = !isDownstreamPromotion && isPromotionWindowClosed(props.stage);
  const rollbackRequired = downgrades.length > 0 && !rollback;

  let promotingTo = stageName || '';

  if (isDownstreamPromotion) {
    promotingTo = [...(dictionaryContext?.subscribersByStage?.[promotingTo] || [])].join(', ');
  }

  return (
    <Drawer
      open={props.visible}
      onClose={props.hide}
      title={
        <Flex align='center'>
          Promote {freightAlias} to {promotingTo}
        </Flex>
      }
      size='large'
      width={'1400px'}
      footer={
        <>
          {downgrades.length > 0 && (
            <Alert
              type='warning'
              showIcon
              message='This promotion is a downgrade'
              description={
                <ul className='m-0 pl-4'>
                  {downgrades.map((d) => (
                    <li key={d}>{d}</li>
                  ))}
                </ul>
              }
            />
          )}
          <Checkbox
            className='mt-3'
            checked={rollback}
            onChange={(e) => setRollback(e.target.checked)}
          >
            This is an intentional rollback (allow promoting an older version)
          </Checkbox>
          <Tooltip
            title={
              windowClosed
                ? promotionWindowClosedMessage(props.stage)
                : rollbackRequired
                  ? 'Confirm the rollback to promote an older version'
                  : undefined
            }
          >
            <Button
              size='large'
              className={classNames(styles['promote-btn'], 'ml-auto mt-5')}
              icon={<FontAwesomeIcon icon={faTruckArrowRight} />}
              onClick={onPromote}
              disabled={windowClosed || rollbackRequired}
              loading={promoteActionMutation.isPending || promoteDownstreamActionMutation.isPending}
            >
              {rollback ? 'Roll back' : 'Promote'}
              {isDownstreamPromotion && ' to downstream'}
            </Button>
          </Tooltip>
        </>
      }
    >
      <div className='-mt-4'>
        <FreightDetails
          freight={props.freight}
          comparison={{ currentFreight: currentFreightOnStage }}
          additionalTabs={promoteTabs.map((data, index) => ({
            children: <data.component freight={props.freight} stage={props.stage} />,
            key: String(data.label + index),
            label: data.label,
            icon: data.icon
          }))}
        />
      </div>
    </Drawer>
  );
};
