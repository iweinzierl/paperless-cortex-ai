import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'package:app/providers/auth_provider.dart';
import 'package:app/screens/dashboard_screen.dart';
import 'package:app/screens/queue_screen.dart';
import 'package:app/services/api_service.dart';
import 'package:app/theme.dart';

class AppShell extends StatefulWidget {
  const AppShell({super.key});

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int _selectedIndex = 0;

  @override
  Widget build(BuildContext context) {
    final auth = context.watch<AuthProvider>();
    final apiService = context.read<ApiService>();

    final pages = [
      DashboardScreen(apiService: apiService, username: auth.username),
      QueueScreen(apiService: apiService),
      AccountScreen(username: auth.username),
    ];

    return Scaffold(
      body: SafeArea(
        child: IndexedStack(index: _selectedIndex, children: pages),
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _selectedIndex,
        onDestinationSelected: (index) {
          setState(() {
            _selectedIndex = index;
          });
        },
        backgroundColor: AppColors.surfaceRaised,
        indicatorColor: AppColors.primary.withValues(alpha: 0.12),
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.space_dashboard_outlined),
            selectedIcon: Icon(Icons.space_dashboard_rounded),
            label: 'Dashboard',
          ),
          NavigationDestination(
            icon: Icon(Icons.inbox_outlined),
            selectedIcon: Icon(Icons.inbox_rounded),
            label: 'Queue',
          ),
          NavigationDestination(
            icon: Icon(Icons.person_outline),
            selectedIcon: Icon(Icons.person_rounded),
            label: 'Account',
          ),
        ],
      ),
    );
  }
}

class AccountScreen extends StatelessWidget {
  const AccountScreen({super.key, required this.username});

  final String? username;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 28),
      children: [
        Text('Account', style: theme.textTheme.headlineSmall),
        const SizedBox(height: 8),
        Text(
          'Signed in as ${username ?? 'Paperless operator'}.',
          style: theme.textTheme.bodyMedium,
        ),
        const SizedBox(height: 20),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _AccountRow(
                  title: 'Backend endpoint',
                  value: ApiService.baseUrl,
                ),
                const SizedBox(height: 14),
                const _AccountRow(
                  title: 'Mobile shell',
                  value: 'Dashboard and queue navigation are active.',
                ),
                const SizedBox(height: 14),
                const _AccountRow(
                  title: 'Next mobile work',
                  value:
                      'Document details, process actions, and configuration flows.',
                ),
                const SizedBox(height: 22),
                ElevatedButton.icon(
                  onPressed: () => context.read<AuthProvider>().logout(),
                  icon: const Icon(Icons.logout_rounded),
                  label: const Text('Sign out'),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _AccountRow extends StatelessWidget {
  const _AccountRow({required this.title, required this.value});

  final String title;
  final String value;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(title.toUpperCase(), style: theme.textTheme.labelMedium),
        const SizedBox(height: 6),
        Text(value, style: theme.textTheme.bodyLarge),
      ],
    );
  }
}
