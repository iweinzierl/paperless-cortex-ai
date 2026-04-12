import 'package:flutter/material.dart';

import 'package:app/models/app_models.dart';
import 'package:app/services/api_service.dart';
import 'package:app/theme.dart';

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({
    super.key,
    required this.apiService,
    required this.username,
  });

  final ApiService apiService;
  final String? username;

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  late Future<_DashboardBundle> _future;

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<_DashboardBundle> _load() async {
    final results = await Future.wait<dynamic>([
      widget.apiService.getDashboard(),
      widget.apiService.getSystemStatus(),
    ]);
    return _DashboardBundle(
      stats: results[0] as DashboardStats,
      status: results[1] as SystemStatusModel,
    );
  }

  Future<void> _refresh() async {
    final future = _load();
    setState(() {
      _future = future;
    });
    await future;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return RefreshIndicator(
      onRefresh: _refresh,
      child: FutureBuilder<_DashboardBundle>(
        future: _future,
        builder: (context, snapshot) {
          if (snapshot.connectionState != ConnectionState.done) {
            return const Center(child: CircularProgressIndicator());
          }

          if (snapshot.hasError) {
            return _LoadErrorView(
              title: 'Dashboard unavailable',
              message: snapshot.error.toString(),
              onRetry: _refresh,
            );
          }

          final data = snapshot.data!;
          return ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(20, 20, 20, 28),
            children: [
              Text('Dashboard', style: theme.textTheme.headlineSmall),
              const SizedBox(height: 6),
              Text(
                'Welcome ${widget.username ?? 'back'}. This mobile shell keeps the queue and system health in reach while we adapt the rest of the workflow for phones.',
                style: theme.textTheme.bodyMedium,
              ),
              const SizedBox(height: 20),
              Row(
                children: [
                  Expanded(
                    child: _MetricCard(
                      title: 'Queued',
                      value: data.stats.queuedCount.toString(),
                      accent: AppColors.primary,
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: _MetricCard(
                      title: 'Avg time',
                      value: _formatDurationMs(data.stats.averageProcessingMs),
                      accent: AppColors.secondary,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 12),
              _MetricCard(
                title: 'Success rate',
                value:
                    '${data.stats.processingSuccessRate.toStringAsFixed(1)}%',
                accent: AppColors.tertiary,
                fullWidth: true,
              ),
              const SizedBox(height: 20),
              Text('System status', style: theme.textTheme.titleLarge),
              const SizedBox(height: 12),
              _StatusCard(
                title: 'Backend',
                status: data.status.backend,
                icon: Icons.cloud_done_outlined,
              ),
              const SizedBox(height: 10),
              _StatusCard(
                title: 'Paperless',
                status: data.status.paperless,
                icon: Icons.inventory_2_outlined,
              ),
              const SizedBox(height: 10),
              _StatusCard(
                title: 'Ollama',
                status: data.status.ollama,
                icon: Icons.memory_outlined,
              ),
              const SizedBox(height: 20),
              Text('Recent runs', style: theme.textTheme.titleLarge),
              const SizedBox(height: 12),
              if (data.stats.recentRuns.isEmpty)
                const _EmptyStateCard(
                  title: 'No recent runs',
                  message:
                      'Queue activity will appear here once documents are processed.',
                )
              else
                ...data.stats.recentRuns
                    .take(5)
                    .map(
                      (item) => Padding(
                        padding: const EdgeInsets.only(bottom: 10),
                        child: _QueueRunCard(item: item),
                      ),
                    ),
            ],
          );
        },
      ),
    );
  }
}

class _DashboardBundle {
  const _DashboardBundle({required this.stats, required this.status});

  final DashboardStats stats;
  final SystemStatusModel status;
}

class _MetricCard extends StatelessWidget {
  const _MetricCard({
    required this.title,
    required this.value,
    required this.accent,
    this.fullWidth = false,
  });

  final String title;
  final String value;
  final Color accent;
  final bool fullWidth;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: EdgeInsets.symmetric(
          horizontal: 18,
          vertical: fullWidth ? 20 : 18,
        ),
        child: Row(
          children: [
            Container(
              width: 12,
              height: 12,
              decoration: BoxDecoration(color: accent, shape: BoxShape.circle),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title.toUpperCase(), style: theme.textTheme.labelMedium),
                  const SizedBox(height: 8),
                  Text(value, style: theme.textTheme.headlineSmall),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _StatusCard extends StatelessWidget {
  const _StatusCard({
    required this.title,
    required this.status,
    required this.icon,
  });

  final String title;
  final DependencyStatusModel status;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final color = _statusColor(status);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: color.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(14),
              ),
              child: Icon(icon, color: color),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title, style: theme.textTheme.bodyLarge),
                  const SizedBox(height: 4),
                  Text(
                    status.message.trim().isEmpty
                        ? _statusLabel(status)
                        : status.message,
                    style: theme.textTheme.bodyMedium,
                  ),
                ],
              ),
            ),
            Text(
              _statusLabel(status),
              style: theme.textTheme.labelMedium?.copyWith(color: color),
            ),
          ],
        ),
      ),
    );
  }
}

class _QueueRunCard extends StatelessWidget {
  const _QueueRunCard({required this.item});

  final QueueItem item;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    item.documentTitle,
                    style: theme.textTheme.bodyLarge,
                  ),
                ),
                _StatusPill(status: item.status),
              ],
            ),
            const SizedBox(height: 8),
            Text(
              item.resultSummary?.trim().isNotEmpty == true
                  ? item.resultSummary!
                  : item.lastError?.trim().isNotEmpty == true
                  ? item.lastError!
                  : 'Processed from ${item.source}.',
              style: theme.textTheme.bodyMedium,
            ),
          ],
        ),
      ),
    );
  }
}

class _StatusPill extends StatelessWidget {
  const _StatusPill({required this.status});

  final String status;

  @override
  Widget build(BuildContext context) {
    final label = status.replaceAll('_', ' ');
    final color = switch (status) {
      'completed' => AppColors.tertiary,
      'partially_completed' => const Color(0xFF9A6700),
      'failed' => AppColors.error,
      'processing' => AppColors.primary,
      _ => AppColors.secondary,
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 12,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }
}

class _EmptyStateCard extends StatelessWidget {
  const _EmptyStateCard({required this.title, required this.message});

  final String title;
  final String message;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(title, style: theme.textTheme.bodyLarge),
            const SizedBox(height: 6),
            Text(message, style: theme.textTheme.bodyMedium),
          ],
        ),
      ),
    );
  }
}

class _LoadErrorView extends StatelessWidget {
  const _LoadErrorView({
    required this.title,
    required this.message,
    required this.onRetry,
  });

  final String title;
  final String message;
  final Future<void> Function() onRetry;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(20),
      children: [
        Card(
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: theme.textTheme.titleLarge),
                const SizedBox(height: 8),
                Text(message, style: theme.textTheme.bodyMedium),
                const SizedBox(height: 18),
                ElevatedButton(onPressed: onRetry, child: const Text('Retry')),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

String _formatDurationMs(double milliseconds) {
  if (milliseconds <= 0) {
    return '0s';
  }
  final seconds = milliseconds / 1000;
  if (seconds < 60) {
    return '${seconds.toStringAsFixed(1)}s';
  }
  final minutes = seconds / 60;
  return '${minutes.toStringAsFixed(1)}m';
}

Color _statusColor(DependencyStatusModel status) {
  if (!status.configured) {
    return AppColors.secondary;
  }
  return status.healthy ? AppColors.tertiary : AppColors.error;
}

String _statusLabel(DependencyStatusModel status) {
  if (!status.configured) {
    return 'not configured';
  }
  return status.healthy ? 'connected' : 'unavailable';
}
