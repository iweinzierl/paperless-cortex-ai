import 'dart:async';

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:provider/provider.dart';
import 'package:webapp/models/models.dart';
import 'package:webapp/providers/auth_provider.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/theme.dart';

class AppLayout extends StatefulWidget {
  final Widget child;

  const AppLayout({super.key, required this.child});

  @override
  State<AppLayout> createState() => _AppLayoutState();
}

class _AppLayoutState extends State<AppLayout> {
  SystemStatusModel? _status;
  Timer? _statusTimer;

  @override
  void initState() {
    super.initState();
    _loadStatus();
    _statusTimer = Timer.periodic(const Duration(seconds: 20), (_) {
      _loadStatus();
    });
  }

  @override
  void dispose() {
    _statusTimer?.cancel();
    super.dispose();
  }

  Future<void> _loadStatus() async {
    try {
      final nextStatus = await context.read<ApiService>().getSystemStatus();
      if (mounted) {
        setState(() {
          _status = nextStatus;
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() {
          _status = null;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: TailwindColors.surface,
      body: Row(
        children: [
          SideNavBar(status: _status),
          Expanded(
            child: Column(
              children: [
                TopNavBar(status: _status),
                Expanded(child: widget.child),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class SideNavBar extends StatelessWidget {
  final SystemStatusModel? status;

  const SideNavBar({super.key, required this.status});

  @override
  Widget build(BuildContext context) {
    final authProvider = context.watch<AuthProvider>();
    final location = GoRouterState.of(context).matchedLocation;
    return Container(
      width: 256,
      color: TailwindColors.surfaceContainerLow,
      padding: const EdgeInsets.symmetric(vertical: 24, horizontal: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Cortex Graphite',
                  style: TextStyle(
                    fontSize: 20,
                    fontWeight: FontWeight.w900,
                    color: TailwindColors.onSurface,
                    letterSpacing: -0.5,
                  ),
                ),
                Text(
                  _dependencySummary('Ollama', status?.ollama),
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: TailwindColors.outline,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 32),
          InkWell(
            onTap: () => context.go('/queue'),
            borderRadius: BorderRadius.circular(12),
            child: Container(
              width: double.infinity,
              padding: const EdgeInsets.symmetric(vertical: 12),
              decoration: BoxDecoration(
                gradient: const LinearGradient(
                  colors: [
                    TailwindColors.primary,
                    TailwindColors.primaryContainer,
                  ],
                ),
                borderRadius: BorderRadius.circular(12),
                boxShadow: [
                  BoxShadow(
                    color: TailwindColors.primary.withValues(alpha: 0.3),
                    blurRadius: 4,
                    offset: const Offset(0, 2),
                  ),
                ],
              ),
              child: const Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(
                    Icons.playlist_add_check,
                    color: TailwindColors.onPrimary,
                    size: 20,
                  ),
                  SizedBox(width: 8),
                  Text(
                    'Review Queue',
                    style: TextStyle(
                      color: TailwindColors.onPrimary,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 32),
          _NavItem(
            icon: Icons.dashboard,
            title: 'Dashboard',
            active: location.startsWith('/dashboard'),
            onTap: () => context.go('/dashboard'),
          ),
          const SizedBox(height: 4),
          _NavItem(
            icon: Icons.queue,
            title: 'Queue',
            active: location.startsWith('/queue'),
            onTap: () => context.go('/queue'),
          ),
          const SizedBox(height: 4),
          _NavItem(
            icon: Icons.settings,
            title: 'Configuration',
            active: location.startsWith('/configuration'),
            onTap: () => context.go('/configuration'),
          ),
          const Spacer(),
          const Divider(color: TailwindColors.outlineVariant),
          const SizedBox(height: 16),
          _NavItem(
            icon: Icons.help_outline,
            title: 'Docs',
            active: location.startsWith('/docs'),
            onTap: () => context.go('/docs'),
          ),
          const SizedBox(height: 4),
          _NavItem(
            icon: Icons.logout,
            title: 'Logout',
            active: false,
            onTap: () async {
              await authProvider.logout();
              if (context.mounted) {
                context.go('/login');
              }
            },
          ),
        ],
      ),
    );
  }

  String _dependencySummary(String name, DependencyStatusModel? dependency) {
    if (dependency == null) {
      return '$name: Checking';
    }
    if (!dependency.configured) {
      return '$name: Not configured';
    }
    return '$name: ${dependency.healthy ? 'Connected' : 'Unavailable'}';
  }
}

class _NavItem extends StatelessWidget {
  final IconData icon;
  final String title;
  final bool active;
  final VoidCallback onTap;

  const _NavItem({
    required this.icon,
    required this.title,
    required this.active,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: active
              ? TailwindColors.primary.withValues(alpha: 0.1)
              : Colors.transparent,
          borderRadius: BorderRadius.circular(8),
          border: active
              ? const Border(
                  left: BorderSide(color: TailwindColors.primary, width: 2),
                )
              : null,
        ),
        child: Row(
          children: [
            Icon(
              icon,
              size: 20,
              color: active ? TailwindColors.primary : TailwindColors.outline,
            ),
            const SizedBox(width: 12),
            Text(
              title,
              style: TextStyle(
                fontSize: 14,
                fontWeight: active ? FontWeight.w600 : FontWeight.w500,
                color: active ? TailwindColors.primary : TailwindColors.outline,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class TopNavBar extends StatelessWidget {
  final SystemStatusModel? status;

  const TopNavBar({super.key, required this.status});

  @override
  Widget build(BuildContext context) {
    final authProvider = context.watch<AuthProvider>();
    return Container(
      height: 64,
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.8),
        border: const Border(
          bottom: BorderSide(color: TailwindColors.surfaceContainerHigh),
        ),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 32),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          SizedBox(
            width: 400,
            child: TextField(
              onSubmitted: (value) {
                final query = value.trim().toLowerCase();
                if (query.isEmpty) {
                  return;
                }
                if ('dashboard'.contains(query)) {
                  context.go('/dashboard');
                  return;
                }
                if ('queue'.contains(query)) {
                  context.go('/queue');
                  return;
                }
                if ('configuration settings config'.contains(query)) {
                  context.go('/configuration');
                  return;
                }
                if ('docs documentation help'.contains(query)) {
                  context.go('/docs');
                  return;
                }
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(
                    content: Text(
                      'No quick navigation target matched that search.',
                    ),
                  ),
                );
              },
              decoration: InputDecoration(
                hintText: 'Jump to dashboard, queue, configuration, or docs...',
                hintStyle: const TextStyle(
                  color: TailwindColors.outline,
                  fontSize: 14,
                ),
                prefixIcon: const Icon(
                  Icons.search,
                  color: TailwindColors.outline,
                  size: 20,
                ),
                filled: true,
                fillColor: TailwindColors.surfaceContainerHighest,
                contentPadding: const EdgeInsets.symmetric(vertical: 0),
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(24),
                  borderSide: BorderSide.none,
                ),
              ),
            ),
          ),
          Row(
            children: [
              IconButton(
                onPressed: () => _showStatusDialog(context),
                icon: const Icon(
                  Icons.notifications_none,
                  color: TailwindColors.outline,
                ),
              ),
              IconButton(
                onPressed: () => context.go('/configuration'),
                icon: const Icon(
                  Icons.dns_outlined,
                  color: TailwindColors.outline,
                ),
              ),
              Container(
                height: 32,
                width: 1,
                color: TailwindColors.surfaceContainerHigh,
                margin: const EdgeInsets.symmetric(horizontal: 8),
              ),
              Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    authProvider.username?.trim().isNotEmpty == true
                        ? authProvider.username!
                        : 'Authenticated User',
                    style: const TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.bold,
                      color: TailwindColors.onSurface,
                    ),
                  ),
                  Text(
                    _paperlessSummary(status?.paperless),
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w500,
                      color: TailwindColors.tertiary,
                    ),
                  ),
                ],
              ),
              const SizedBox(width: 8),
              Container(
                width: 32,
                height: 32,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: TailwindColors.surfaceContainerHighest,
                  border: Border.all(color: TailwindColors.outlineVariant),
                ),
                child: const Icon(
                  Icons.person,
                  size: 20,
                  color: TailwindColors.outline,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  String _paperlessSummary(DependencyStatusModel? dependency) {
    if (dependency == null) {
      return 'Paperless: Checking';
    }
    if (!dependency.configured) {
      return 'Paperless: Not configured';
    }
    return 'Paperless: ${dependency.healthy ? 'Connected' : 'Unavailable'}';
  }

  Future<void> _showStatusDialog(BuildContext context) async {
    await showDialog<void>(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          title: const Text('System status'),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _StatusRow(label: 'Backend', dependency: status?.backend),
              const SizedBox(height: 12),
              _StatusRow(label: 'Paperless', dependency: status?.paperless),
              const SizedBox(height: 12),
              _StatusRow(label: 'Ollama', dependency: status?.ollama),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(),
              child: const Text('Close'),
            ),
          ],
        );
      },
    );
  }
}

class _StatusRow extends StatelessWidget {
  final String label;
  final DependencyStatusModel? dependency;

  const _StatusRow({required this.label, required this.dependency});

  @override
  Widget build(BuildContext context) {
    final color = dependency == null
        ? TailwindColors.outline
        : !dependency!.configured
        ? TailwindColors.secondary
        : dependency!.healthy
        ? TailwindColors.tertiary
        : TailwindColors.error;
    final message = dependency?.message ?? 'Status unavailable';
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Container(
          width: 8,
          height: 8,
          margin: const EdgeInsets.only(top: 4),
          decoration: BoxDecoration(color: color, shape: BoxShape.circle),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            '$label: $message',
            style: const TextStyle(
              fontSize: 13,
              color: TailwindColors.onSurfaceVariant,
            ),
          ),
        ),
      ],
    );
  }
}
