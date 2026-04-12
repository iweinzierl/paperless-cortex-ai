import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:webapp/models/models.dart';
import 'package:webapp/theme.dart';

class QueueStageDefinition {
  final String key;
  final String label;

  const QueueStageDefinition({required this.key, required this.label});
}

const List<QueueStageDefinition> queueStageDefinitions = [
  QueueStageDefinition(key: 'extract_text', label: 'OCR / Text'),
  QueueStageDefinition(key: 'created_date', label: 'Created Date'),
  QueueStageDefinition(key: 'correspondent', label: 'Correspondent'),
  QueueStageDefinition(key: 'document_type', label: 'Doc Type'),
  QueueStageDefinition(key: 'tags', label: 'Tags'),
  QueueStageDefinition(key: 'title', label: 'Title'),
];

Set<String> configuredQueueStageKeys(ProcessConfig process) {
  final configured = <String>{'extract_text'};
  if (process.processCreatedDateTag.trim().isNotEmpty) {
    configured.add('created_date');
  }
  if (process.processCorrespondentTag.trim().isNotEmpty) {
    configured.add('correspondent');
  }
  if (process.processDocumentTypeTag.trim().isNotEmpty) {
    configured.add('document_type');
  }
  if (process.processDocumentTagsTag.trim().isNotEmpty) {
    configured.add('tags');
  }
  if (process.processTitleTag.trim().isNotEmpty) {
    configured.add('title');
  }
  return configured;
}

List<ProcessingStageProgress> buildQueueStageOverview(
  QueueItem item,
  Set<String> configuredStageKeys,
) {
  final resultStages = item.processingResult?.stages;
  final requestedKeys = item.requestedStageKeys.isNotEmpty
      ? item.requestedStageKeys.toSet()
      : configuredStageKeys;

  if (resultStages != null && resultStages.isNotEmpty) {
    final stageByKey = {for (final stage in resultStages) stage.key: stage};
    return queueStageDefinitions
        .map((definition) {
          final resolved = stageByKey[definition.key];
          if (resolved != null) {
            return ProcessingStageProgress(
              key: definition.key,
              label: definition.label,
              status: resolved.status,
              detail: resolved.detail,
              usedModel: resolved.usedModel,
            );
          }

          final isRequested = requestedKeys.contains(definition.key);
          return ProcessingStageProgress(
            key: definition.key,
            label: definition.label,
            status: isRequested ? 'pending' : 'skipped',
          );
        })
        .toList(growable: false);
  }

  final inferredStatus = item.status == 'completed' ? 'completed' : 'pending';
  return queueStageDefinitions
      .map((definition) {
        final isRequested = requestedKeys.contains(definition.key);
        return ProcessingStageProgress(
          key: definition.key,
          label: definition.label,
          status: isRequested ? inferredStatus : 'skipped',
        );
      })
      .toList(growable: false);
}

class QueueStageLegend extends StatelessWidget {
  const QueueStageLegend({super.key});

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: 12,
      runSpacing: 8,
      crossAxisAlignment: WrapCrossAlignment.center,
      children: const [
        _LegendLabel('Step states:'),
        _LegendItem(label: 'Completed', status: 'completed'),
        _LegendItem(label: 'Running', status: 'running'),
        _LegendItem(label: 'Pending', status: 'pending'),
        _LegendItem(label: 'Failed', status: 'failed'),
        _LegendItem(label: 'Not configured', status: 'skipped'),
      ],
    );
  }
}

class QueueStageGrid extends StatelessWidget {
  final QueueItem item;
  final Set<String> configuredStageKeys;
  final double minColumnWidth;

  const QueueStageGrid({
    super.key,
    required this.item,
    required this.configuredStageKeys,
    this.minColumnWidth = 108,
  });

  @override
  Widget build(BuildContext context) {
    final stages = buildQueueStageOverview(item, configuredStageKeys);
    return LayoutBuilder(
      builder: (context, constraints) {
        final compact = constraints.maxWidth < (minColumnWidth * 4.5);
        return _StageStepper(stages: stages, compact: compact);
      },
    );
  }
}

class _LegendLabel extends StatelessWidget {
  final String label;

  const _LegendLabel(this.label);

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: const TextStyle(
        fontSize: 11,
        fontWeight: FontWeight.w700,
        color: TailwindColors.onSurfaceVariant,
      ),
    );
  }
}

class _LegendItem extends StatelessWidget {
  final String label;
  final String status;

  const _LegendItem({required this.label, required this.status});

  @override
  Widget build(BuildContext context) {
    final style = _stageVisualStyle(status);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        _StageCircle(status: status, style: style, size: 18),
        const SizedBox(width: 6),
        Text(
          label,
          style: const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w700,
            color: TailwindColors.onSurfaceVariant,
          ),
        ),
      ],
    );
  }
}

class _StageStepper extends StatelessWidget {
  final List<ProcessingStageProgress> stages;
  final bool compact;

  const _StageStepper({required this.stages, required this.compact});

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (var index = 0; index < stages.length; index++)
          Expanded(
            child: _StageStep(
              stage: stages[index],
              compact: compact,
              isLast: index == stages.length - 1,
              connectorColor: _connectorColor(
                current: stages[index],
                next: index + 1 < stages.length ? stages[index + 1] : null,
              ),
            ),
          ),
      ],
    );
  }
}

class _StageStep extends StatelessWidget {
  final ProcessingStageProgress stage;
  final bool compact;
  final bool isLast;
  final Color connectorColor;

  const _StageStep({
    required this.stage,
    required this.compact,
    required this.isLast,
    required this.connectorColor,
  });

  @override
  Widget build(BuildContext context) {
    final style = _stageVisualStyle(stage.status);
    final tooltip = switch (stage.status) {
      'completed' => '${stage.label}: completed',
      'running' => '${stage.label}: running',
      'failed' => '${stage.label}: failed',
      'skipped' => '${stage.label}: not configured for this item',
      _ => '${stage.label}: pending',
    };
    final detail = stage.detail?.trim();

    return Tooltip(
      message: detail?.isNotEmpty == true ? '$tooltip\n$detail' : tooltip,
      child: Padding(
        padding: EdgeInsets.only(right: isLast ? 0 : 4),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                _StageCircle(status: stage.status, style: style),
                if (!isLast) ...[
                  const SizedBox(width: 8),
                  Expanded(
                    child: Container(
                      height: 3,
                      decoration: BoxDecoration(
                        color: connectorColor,
                        borderRadius: BorderRadius.circular(999),
                      ),
                    ),
                  ),
                ],
              ],
            ),
            const SizedBox(height: 8),
            Padding(
              padding: const EdgeInsets.only(right: 8),
              child: Text(
                _stageDisplayLabel(stage, compact: compact),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  fontSize: compact ? 9 : 10,
                  fontWeight: FontWeight.w800,
                  color: style.labelColor,
                  letterSpacing: 0.3,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _StageCircle extends StatelessWidget {
  final String status;
  final _StageVisualStyle style;
  final double size;

  const _StageCircle({
    required this.status,
    required this.style,
    this.size = 24,
  });

  @override
  Widget build(BuildContext context) {
    final icon = _stageIcon(status);
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: style.fillColor,
        shape: BoxShape.circle,
        border: Border.all(color: style.ringColor, width: style.borderWidth),
      ),
      child: icon == null
          ? null
          : Center(
              child: status == 'skipped'
                  ? Transform.rotate(
                      angle: -math.pi / 4,
                      child: Icon(
                        icon,
                        size: size * 0.58,
                        color: style.iconColor,
                      ),
                    )
                  : Icon(icon, size: size * 0.58, color: style.iconColor),
            ),
    );
  }
}

class _StageVisualStyle {
  final Color fillColor;
  final Color ringColor;
  final Color iconColor;
  final Color labelColor;
  final double borderWidth;

  const _StageVisualStyle({
    required this.fillColor,
    required this.ringColor,
    required this.iconColor,
    required this.labelColor,
    required this.borderWidth,
  });
}

_StageVisualStyle _stageVisualStyle(String status) {
  switch (status) {
    case 'completed':
      return const _StageVisualStyle(
        fillColor: TailwindColors.tertiary,
        ringColor: TailwindColors.tertiary,
        iconColor: TailwindColors.onTertiary,
        labelColor: TailwindColors.onSurfaceVariant,
        borderWidth: 1.5,
      );
    case 'running':
      return const _StageVisualStyle(
        fillColor: TailwindColors.surfaceContainerLowest,
        ringColor: TailwindColors.primary,
        iconColor: TailwindColors.primary,
        labelColor: TailwindColors.primary,
        borderWidth: 2,
      );
    case 'failed':
      return const _StageVisualStyle(
        fillColor: TailwindColors.error,
        ringColor: TailwindColors.error,
        iconColor: TailwindColors.onError,
        labelColor: TailwindColors.error,
        borderWidth: 1.5,
      );
    case 'skipped':
      return const _StageVisualStyle(
        fillColor: TailwindColors.surfaceContainerHighest,
        ringColor: TailwindColors.outlineVariant,
        iconColor: TailwindColors.surfaceContainerLowest,
        labelColor: TailwindColors.outline,
        borderWidth: 1.5,
      );
    default:
      return const _StageVisualStyle(
        fillColor: TailwindColors.surfaceContainerLowest,
        ringColor: TailwindColors.outlineVariant,
        iconColor: Colors.transparent,
        labelColor: TailwindColors.onSurfaceVariant,
        borderWidth: 1.5,
      );
  }
}

Color _connectorColor({
  required ProcessingStageProgress current,
  required ProcessingStageProgress? next,
}) {
  if (next?.isFailed == true) {
    return TailwindColors.error;
  }
  if (current.isCompleted) {
    return TailwindColors.tertiary;
  }
  return TailwindColors.outlineVariant;
}

String _stageDisplayLabel(
  ProcessingStageProgress stage, {
  required bool compact,
}) {
  switch (stage.key) {
    case 'extract_text':
      return 'OCR';
    case 'created_date':
      return 'DATE';
    case 'correspondent':
      return compact ? 'CORR' : 'CORR.';
    case 'document_type':
      return 'TYPE';
    case 'tags':
      return 'TAGS';
    case 'title':
      return 'TITLE';
    default:
      return stage.label.toUpperCase();
  }
}

IconData? _stageIcon(String status) {
  switch (status) {
    case 'completed':
      return Icons.check_circle;
    case 'running':
      return Icons.autorenew;
    case 'failed':
      return Icons.close;
    case 'skipped':
      return Icons.remove;
    default:
      return null;
  }
}
