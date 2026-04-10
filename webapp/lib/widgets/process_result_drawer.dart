import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:webapp/models/models.dart';
import 'package:webapp/theme.dart';

const processResultDrawerTransitionDuration = Duration(milliseconds: 1000);
const processResultDrawerSlideCurve = Curves.easeInOutCubicEmphasized;

class ProcessResultDrawer extends StatelessWidget {
  final QueueItem? item;
  final bool isOpen;
  final VoidCallback onClose;
  final VoidCallback? onRemove;
  final bool isRemoving;

  const ProcessResultDrawer({
    super.key,
    required this.item,
    required this.isOpen,
    required this.onClose,
    this.onRemove,
    this.isRemoving = false,
  });

  @override
  Widget build(BuildContext context) {
    if (item == null) {
      return const SizedBox.shrink();
    }

    final size = MediaQuery.sizeOf(context);
    final drawerWidth = size.width >= 1480
        ? 560.0
        : size.width >= 1180
        ? 500.0
        : size.width >= 900
        ? 440.0
        : size.width * 0.92;

    return IgnorePointer(
      ignoring: !isOpen,
      child: Stack(
        children: [
          Positioned.fill(
            child: GestureDetector(
              onTap: onClose,
              child: TweenAnimationBuilder<double>(
                tween: Tween<double>(begin: 0, end: isOpen ? 1 : 0),
                duration: processResultDrawerTransitionDuration,
                curve: Curves.easeOutCubic,
                builder: (context, value, child) {
                  return ClipRect(
                    child: BackdropFilter(
                      filter: ImageFilter.blur(
                        sigmaX: 10 * value,
                        sigmaY: 10 * value,
                      ),
                      child: Container(
                        decoration: BoxDecoration(
                          color: Color.lerp(
                            Colors.transparent,
                            const Color(0x7A0B1220),
                            value,
                          ),
                        ),
                      ),
                    ),
                  );
                },
              ),
            ),
          ),
          Align(
            alignment: Alignment.centerRight,
            child: TweenAnimationBuilder<double>(
              tween: Tween<double>(begin: 0, end: isOpen ? 1 : 0),
              duration: processResultDrawerTransitionDuration,
              curve: processResultDrawerSlideCurve,
              builder: (context, value, child) {
                final slideOffset = Offset(1.08 * (1 - value), 0);
                final scale = 0.975 + (0.025 * value);
                return FractionalTranslation(
                  translation: slideOffset,
                  child: Transform.scale(
                    scale: scale,
                    alignment: Alignment.centerRight,
                    child: Opacity(opacity: value, child: child),
                  ),
                );
              },
              child: DecoratedBox(
                decoration: const BoxDecoration(
                  boxShadow: [
                    BoxShadow(
                      color: Color(0x30000000),
                      blurRadius: 40,
                      offset: Offset(-12, 0),
                    ),
                  ],
                ),
                child: Container(
                  width: drawerWidth,
                  height: double.infinity,
                  decoration: BoxDecoration(
                    color: TailwindColors.surfaceContainerLowest,
                    border: Border(
                      left: BorderSide(
                        color: TailwindColors.primary.withValues(alpha: 0.14),
                      ),
                    ),
                  ),
                  child: Stack(
                    children: [
                      Positioned(
                        left: 0,
                        top: 0,
                        bottom: 0,
                        child: IgnorePointer(
                          child: Container(
                            width: 18,
                            decoration: BoxDecoration(
                              gradient: LinearGradient(
                                begin: Alignment.centerLeft,
                                end: Alignment.centerRight,
                                colors: [
                                  TailwindColors.primary.withValues(
                                    alpha: 0.10,
                                  ),
                                  Colors.transparent,
                                ],
                              ),
                            ),
                          ),
                        ),
                      ),
                      _DrawerBody(
                        item: item!,
                        onClose: onClose,
                        onRemove: onRemove,
                        isRemoving: isRemoving,
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _DrawerBody extends StatelessWidget {
  final QueueItem item;
  final VoidCallback onClose;
  final VoidCallback? onRemove;
  final bool isRemoving;

  const _DrawerBody({
    required this.item,
    required this.onClose,
    this.onRemove,
    required this.isRemoving,
  });

  @override
  Widget build(BuildContext context) {
    final result = item.processingResult;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          padding: const EdgeInsets.fromLTRB(24, 20, 16, 20),
          decoration: const BoxDecoration(
            color: TailwindColors.surfaceContainerLow,
            border: Border(
              bottom: BorderSide(color: TailwindColors.surfaceContainerHigh),
            ),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.documentTitle,
                      style: const TextStyle(
                        fontSize: 22,
                        fontWeight: FontWeight.w800,
                        color: TailwindColors.onSurface,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      crossAxisAlignment: WrapCrossAlignment.center,
                      children: [
                        _StatusPill(status: item.status),
                        _MetaPill(label: 'Queue #${item.id}'),
                        _MetaPill(label: item.source.toUpperCase()),
                      ],
                    ),
                  ],
                ),
              ),
              Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (onRemove != null && item.canRemoveFromQueue)
                    IconButton(
                      onPressed: isRemoving ? null : onRemove,
                      icon: isRemoving
                          ? const SizedBox(
                              width: 20,
                              height: 20,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Icon(Icons.delete_outline, size: 22),
                      color: TailwindColors.error,
                      tooltip: 'Remove from queue',
                    ),
                  IconButton(
                    onPressed: onClose,
                    icon: const Icon(Icons.close),
                    color: TailwindColors.onSurfaceVariant,
                    tooltip: 'Close',
                  ),
                ],
              ),
            ],
          ),
        ),
        Expanded(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _buildOutcomeSummary(result),
                const SizedBox(height: 20),
                _SectionCard(
                  title: 'Timing',
                  child: Wrap(
                    spacing: 24,
                    runSpacing: 12,
                    children: [
                      _DetailMetric(
                        label: 'Requested',
                        value: _formatTimestamp(item.requestedAt),
                      ),
                      _DetailMetric(
                        label: 'Completed',
                        value: item.completedAt == null
                            ? 'Not completed'
                            : _formatTimestamp(item.completedAt!),
                      ),
                      _DetailMetric(
                        label: 'Processing Time',
                        value: _formatDurationMs(item.processingDurationMs),
                      ),
                    ],
                  ),
                ),
                if (result != null) ...[
                  const SizedBox(height: 20),
                  _SectionCard(
                    title: 'Stage Overview',
                    child: Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: result.requestedStages
                          .map((stage) => _StagePill(stage: stage))
                          .toList(),
                    ),
                  ),
                  const SizedBox(height: 20),
                  _buildSuggestionSection(
                    title: 'Correspondent',
                    stage: result.correspondent,
                    currentLabel: _documentEntityLabel(
                      name: result.document?.correspondentName,
                      id: result.document?.correspondentId,
                    ),
                    selectedLabel: _payloadString(
                      result.correspondent.payload,
                      'correspondent_name',
                    ),
                    suggestedLabel: _payloadString(
                      result.correspondent.payload,
                      'suggested_new_correspondent',
                    ),
                  ),
                  const SizedBox(height: 20),
                  _buildSuggestionSection(
                    title: 'Document Type',
                    stage: result.documentType,
                    currentLabel: _documentEntityLabel(
                      name: result.document?.documentTypeName,
                      id: result.document?.documentTypeId,
                    ),
                    selectedLabel: _payloadString(
                      result.documentType.payload,
                      'document_type_name',
                    ),
                    suggestedLabel: _payloadString(
                      result.documentType.payload,
                      'suggested_new_document_type',
                    ),
                  ),
                  const SizedBox(height: 20),
                  _buildTagSection(result),
                  const SizedBox(height: 20),
                  _buildExtractionSection(result),
                  if (result.notes.isNotEmpty) ...[
                    const SizedBox(height: 20),
                    _SectionCard(
                      title: 'Notes',
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: result.notes
                            .map(
                              (note) => Padding(
                                padding: const EdgeInsets.only(bottom: 8),
                                child: Row(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    const Padding(
                                      padding: EdgeInsets.only(top: 6),
                                      child: Icon(
                                        Icons.circle,
                                        size: 6,
                                        color: TailwindColors.outline,
                                      ),
                                    ),
                                    const SizedBox(width: 10),
                                    Expanded(
                                      child: Text(
                                        note,
                                        style: const TextStyle(
                                          fontSize: 13,
                                          color:
                                              TailwindColors.onSurfaceVariant,
                                          height: 1.4,
                                        ),
                                      ),
                                    ),
                                  ],
                                ),
                              ),
                            )
                            .toList(),
                      ),
                    ),
                  ],
                ] else ...[
                  const SizedBox(height: 20),
                  _SectionCard(
                    title: 'Process Details',
                    child: Text(
                      item.resultPayload?.trim().isNotEmpty == true
                          ? item.resultPayload!
                          : 'No structured result payload was available for this item.',
                      style: const TextStyle(
                        fontSize: 13,
                        color: TailwindColors.onSurfaceVariant,
                        height: 1.5,
                      ),
                    ),
                  ),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildOutcomeSummary(ProcessingResultModel? result) {
    final isFailed = item.status == 'failed';
    final isPartial = item.status == 'partially_completed';
    final summary = item.detailSummary;
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: isFailed
            ? TailwindColors.errorContainer
            : isPartial
            ? TailwindColors.secondaryContainer
            : TailwindColors.primaryFixed.withValues(alpha: 0.40),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            isFailed
                ? 'Failure Summary'
                : isPartial
                ? 'Partial Result Summary'
                : 'Result Summary',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w800,
              letterSpacing: 0.8,
              color: isFailed
                  ? TailwindColors.onErrorContainer
                  : isPartial
                  ? TailwindColors.onSecondaryContainer
                  : TailwindColors.onPrimaryFixedVariant,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            summary,
            style: TextStyle(
              fontSize: 14,
              height: 1.45,
              color: isFailed
                  ? TailwindColors.onErrorContainer
                  : isPartial
                  ? TailwindColors.onSecondaryContainer
                  : TailwindColors.onSurface,
            ),
          ),
          if (result?.failureSummary != null &&
              result!.failureSummary != item.lastError &&
              result.failureSummary != item.resultSummary) ...[
            const SizedBox(height: 10),
            Text(
              result.failureSummary!,
              style: const TextStyle(
                fontSize: 12,
                height: 1.4,
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildExtractionSection(ProcessingResultModel result) {
    final extraction = result.extraction;
    return _SectionCard(
      title: 'Extraction',
      trailing: _StageBadge(status: extraction.status),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 24,
            runSpacing: 12,
            children: [
              _DetailMetric(
                label: 'Source',
                value: extraction.source.isEmpty ? '--' : extraction.source,
              ),
              _DetailMetric(
                label: 'Model',
                value: extraction.usedModel.isEmpty
                    ? '--'
                    : extraction.usedModel,
              ),
              _DetailMetric(
                label: 'Text Length',
                value: extraction.textLength > 0
                    ? '${extraction.textLength} chars'
                    : '--',
              ),
            ],
          ),
          if (extraction.error.isNotEmpty) ...[
            const SizedBox(height: 14),
            _CalloutText(text: extraction.error, isError: true),
          ],
          if (result.extractionPreview != null) ...[
            const SizedBox(height: 14),
            const Text(
              'Preview',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: TailwindColors.onSurface,
              ),
            ),
            const SizedBox(height: 8),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: TailwindColors.surfaceContainerLow,
                borderRadius: BorderRadius.circular(12),
              ),
              child: SelectableText(
                result.extractionPreview!,
                style: const TextStyle(
                  fontSize: 12,
                  height: 1.5,
                  color: TailwindColors.onSurfaceVariant,
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildSuggestionSection({
    required String title,
    required SuggestionStageModel stage,
    String? currentLabel,
    String? selectedLabel,
    String? suggestedLabel,
  }) {
    final comparison = _buildSuggestionComparison(
      currentLabel: currentLabel,
      selectedLabel: selectedLabel,
      suggestedLabel: suggestedLabel,
    );

    return _SectionCard(
      title: title,
      trailing: _StageBadge(status: stage.status),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 24,
            runSpacing: 12,
            children: [
              _DetailMetric(
                label: 'Current in Paperless',
                value: currentLabel == null || currentLabel.isEmpty
                    ? '--'
                    : currentLabel,
              ),
              _DetailMetric(
                label: 'LLM Selected',
                value: selectedLabel == null || selectedLabel.isEmpty
                    ? '--'
                    : selectedLabel,
              ),
              _DetailMetric(
                label: 'LLM Suggested New',
                value: suggestedLabel == null || suggestedLabel.isEmpty
                    ? '--'
                    : suggestedLabel,
              ),
              _DetailMetric(
                label: 'Confidence',
                value: stage.confidence.isEmpty ? '--' : stage.confidence,
              ),
              _DetailMetric(
                label: 'Model',
                value: stage.usedModel.isEmpty ? '--' : stage.usedModel,
              ),
            ],
          ),
          if (comparison != null) ...[
            const SizedBox(height: 14),
            _ComparisonIndicator(comparison: comparison),
          ],
          if (stage.error.isNotEmpty) ...[
            const SizedBox(height: 14),
            _CalloutText(text: stage.error, isError: true),
          ],
          if (stage.reasoning.isNotEmpty) ...[
            const SizedBox(height: 14),
            const Text(
              'Reasoning',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: TailwindColors.onSurface,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              stage.reasoning,
              style: const TextStyle(
                fontSize: 13,
                height: 1.5,
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildTagSection(ProcessingResultModel result) {
    final stage = result.tags;
    final selectedTags = _payloadStringList(stage.payload, 'tag_names');
    final suggestedTags = _payloadStringList(
      stage.payload,
      'suggested_new_tags',
    );

    return _SectionCard(
      title: 'Tags',
      trailing: _StageBadge(status: stage.status),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Wrap(
            spacing: 24,
            runSpacing: 12,
            children: [
              _DetailMetric(
                label: 'Confidence',
                value: stage.confidence.isEmpty ? '--' : stage.confidence,
              ),
              _DetailMetric(
                label: 'Model',
                value: stage.usedModel.isEmpty ? '--' : stage.usedModel,
              ),
            ],
          ),
          if (selectedTags.isNotEmpty) ...[
            const SizedBox(height: 14),
            const Text(
              'Selected Tags',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: TailwindColors.onSurface,
              ),
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: selectedTags
                  .map((tag) => _MetaPill(label: tag))
                  .toList(),
            ),
          ],
          if (suggestedTags.isNotEmpty) ...[
            const SizedBox(height: 14),
            const Text(
              'Suggested New Tags',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: TailwindColors.onSurface,
              ),
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: suggestedTags
                  .map(
                    (tag) => _MetaPill(
                      label: tag,
                      backgroundColor: TailwindColors.primaryFixed,
                      foregroundColor: TailwindColors.onPrimaryFixedVariant,
                    ),
                  )
                  .toList(),
            ),
          ],
          if (stage.error.isNotEmpty) ...[
            const SizedBox(height: 14),
            _CalloutText(text: stage.error, isError: true),
          ],
          if (stage.reasoning.isNotEmpty) ...[
            const SizedBox(height: 14),
            const Text(
              'Reasoning',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: TailwindColors.onSurface,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              stage.reasoning,
              style: const TextStyle(
                fontSize: 13,
                height: 1.5,
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ],
        ],
      ),
    );
  }

  static String _formatTimestamp(DateTime value) {
    return DateFormat('MMM d, yyyy HH:mm:ss').format(value);
  }

  static String _formatDurationMs(int? durationMs) {
    if (durationMs == null || durationMs <= 0) {
      return '--';
    }

    final duration = Duration(milliseconds: durationMs);
    if (duration.inHours > 0) {
      return '${duration.inHours}h ${duration.inMinutes.remainder(60)}m ${duration.inSeconds.remainder(60)}s';
    }
    if (duration.inMinutes > 0) {
      return '${duration.inMinutes}m ${duration.inSeconds.remainder(60)}s';
    }
    return '${(durationMs / 1000).toStringAsFixed(2)}s';
  }

  static String? _payloadString(Map<String, dynamic>? payload, String key) {
    final value = payload?[key];
    if (value == null) {
      return null;
    }
    final text = value.toString().trim();
    return text.isEmpty ? null : text;
  }

  static List<String> _payloadStringList(
    Map<String, dynamic>? payload,
    String key,
  ) {
    final value = payload?[key];
    if (value is! List) {
      return const [];
    }
    return value
        .map((entry) => entry.toString().trim())
        .where((entry) => entry.isNotEmpty)
        .toList(growable: false);
  }

  static String? _documentEntityLabel({String? name, int? id}) {
    final trimmedName = name?.trim() ?? '';
    if (trimmedName.isNotEmpty) {
      return trimmedName;
    }
    if (id != null && id > 0) {
      return 'ID $id';
    }
    return null;
  }

  static _SuggestionComparison? _buildSuggestionComparison({
    String? currentLabel,
    String? selectedLabel,
    String? suggestedLabel,
  }) {
    final current = currentLabel?.trim() ?? '';
    final selected = selectedLabel?.trim() ?? '';
    final suggested = suggestedLabel?.trim() ?? '';

    if (selected.isEmpty && suggested.isEmpty) {
      return null;
    }

    final proposed = selected.isNotEmpty ? selected : suggested;
    final currentKey = current.toLowerCase();
    final proposedKey = proposed.toLowerCase();
    final matchesCurrent =
        currentKey.isNotEmpty &&
        proposedKey.isNotEmpty &&
        currentKey == proposedKey;

    if (matchesCurrent) {
      return const _SuggestionComparison(
        label: 'Matches current Paperless value',
        isDifferent: false,
      );
    }

    if (current.isEmpty) {
      return const _SuggestionComparison(
        label: 'Paperless has no current value; the LLM suggested setting one',
        isDifferent: true,
      );
    }

    return _SuggestionComparison(
      label: 'Differs from current Paperless value: $current',
      isDifferent: true,
    );
  }
}

class _SuggestionComparison {
  final String label;
  final bool isDifferent;

  const _SuggestionComparison({required this.label, required this.isDifferent});
}

class _SectionCard extends StatelessWidget {
  final String title;
  final Widget child;
  final Widget? trailing;

  const _SectionCard({required this.title, required this.child, this.trailing});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: TailwindColors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: TailwindColors.surfaceContainerHigh),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  title,
                  style: const TextStyle(
                    fontSize: 16,
                    fontWeight: FontWeight.w800,
                    color: TailwindColors.onSurface,
                  ),
                ),
              ),
              if (trailing != null) trailing!,
            ],
          ),
          const SizedBox(height: 14),
          child,
        ],
      ),
    );
  }
}

class _StatusPill extends StatelessWidget {
  final String status;

  const _StatusPill({required this.status});

  @override
  Widget build(BuildContext context) {
    final tone = _statusTone(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
      decoration: BoxDecoration(
        color: tone.background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        status.toUpperCase(),
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w800,
          letterSpacing: 0.8,
          color: tone.foreground,
        ),
      ),
    );
  }
}

class _StageBadge extends StatelessWidget {
  final String status;

  const _StageBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    final tone = _statusTone(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: tone.background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        status.toUpperCase(),
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w800,
          letterSpacing: 0.7,
          color: tone.foreground,
        ),
      ),
    );
  }
}

class _StagePill extends StatelessWidget {
  final ProcessingStageProgress stage;

  const _StagePill({required this.stage});

  @override
  Widget build(BuildContext context) {
    final tone = _statusTone(stage.status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: tone.background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(_statusIcon(stage.status), size: 14, color: tone.foreground),
          const SizedBox(width: 6),
          Text(
            stage.label,
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: tone.foreground,
            ),
          ),
        ],
      ),
    );
  }
}

class _MetaPill extends StatelessWidget {
  final String label;
  final Color? backgroundColor;
  final Color? foregroundColor;

  const _MetaPill({
    required this.label,
    this.backgroundColor,
    this.foregroundColor,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
      decoration: BoxDecoration(
        color: backgroundColor ?? TailwindColors.surfaceContainerHigh,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w700,
          color: foregroundColor ?? TailwindColors.onSurfaceVariant,
        ),
      ),
    );
  }
}

class _DetailMetric extends StatelessWidget {
  final String label;
  final String value;

  const _DetailMetric({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          label.toUpperCase(),
          style: const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w700,
            letterSpacing: 0.8,
            color: TailwindColors.outline,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          value,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: TailwindColors.onSurface,
          ),
        ),
      ],
    );
  }
}

class _CalloutText extends StatelessWidget {
  final String text;
  final bool isError;

  const _CalloutText({required this.text, required this.isError});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: isError
            ? TailwindColors.errorContainer
            : TailwindColors.surfaceContainerLow,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        text,
        style: TextStyle(
          fontSize: 13,
          height: 1.45,
          color: isError
              ? TailwindColors.onErrorContainer
              : TailwindColors.onSurfaceVariant,
        ),
      ),
    );
  }
}

class _ComparisonIndicator extends StatelessWidget {
  final _SuggestionComparison comparison;

  const _ComparisonIndicator({required this.comparison});

  @override
  Widget build(BuildContext context) {
    final isDifferent = comparison.isDifferent;
    final background = isDifferent
        ? TailwindColors.secondaryContainer
        : TailwindColors.tertiary.withValues(alpha: 0.16);
    final foreground = isDifferent
        ? TailwindColors.onSecondaryContainer
        : TailwindColors.tertiary;

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            isDifferent ? Icons.compare_arrows : Icons.check_circle_outline,
            size: 16,
            color: foreground,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              comparison.label,
              style: TextStyle(
                fontSize: 12,
                height: 1.4,
                color: foreground,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

({Color background, Color foreground}) _statusTone(String status) {
  switch (status) {
    case 'completed':
      return (
        background: TailwindColors.tertiary.withValues(alpha: 0.16),
        foreground: TailwindColors.tertiary,
      );
    case 'partially_completed':
      return (
        background: TailwindColors.secondaryContainer,
        foreground: TailwindColors.onSecondaryContainer,
      );
    case 'failed':
      return (
        background: TailwindColors.errorContainer,
        foreground: TailwindColors.onErrorContainer,
      );
    case 'running':
    case 'processing':
      return (
        background: TailwindColors.primaryFixed,
        foreground: TailwindColors.onPrimaryFixedVariant,
      );
    case 'pending':
      return (
        background: TailwindColors.surfaceContainerHigh,
        foreground: TailwindColors.onSurfaceVariant,
      );
    default:
      return (
        background: TailwindColors.surfaceContainerHigh,
        foreground: TailwindColors.onSurfaceVariant,
      );
  }
}

IconData _statusIcon(String status) {
  switch (status) {
    case 'completed':
      return Icons.check_circle;
    case 'partially_completed':
      return Icons.warning_amber_rounded;
    case 'failed':
      return Icons.error;
    case 'running':
    case 'processing':
      return Icons.autorenew;
    default:
      return Icons.schedule;
  }
}
