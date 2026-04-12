import 'package:flutter/material.dart';

class AppColors {
  static const Color primary = Color(0xFF0051AE);
  static const Color primaryStrong = Color(0xFF0969DA);
  static const Color onPrimary = Color(0xFFFFFFFF);
  static const Color secondary = Color(0xFF5A5F66);
  static const Color tertiary = Color(0xFF006326);
  static const Color surface = Color(0xFFF8F9FB);
  static const Color surfaceRaised = Color(0xFFFFFFFF);
  static const Color surfaceMuted = Color(0xFFF1F4F8);
  static const Color ink = Color(0xFF191C1E);
  static const Color inkMuted = Color(0xFF5B6270);
  static const Color outline = Color(0xFFC9CED8);
  static const Color error = Color(0xFFBA1A1A);
  static const Color errorSoft = Color(0xFFFFDAD6);
}

ThemeData buildAppTheme() {
  const colorScheme = ColorScheme.light(
    primary: AppColors.primary,
    onPrimary: AppColors.onPrimary,
    secondary: AppColors.secondary,
    tertiary: AppColors.tertiary,
    error: AppColors.error,
    surface: AppColors.surface,
    onSurface: AppColors.ink,
    outline: AppColors.outline,
  );

  final base = ThemeData(
    useMaterial3: true,
    colorScheme: colorScheme,
    scaffoldBackgroundColor: AppColors.surface,
  );

  return base.copyWith(
    textTheme: base.textTheme.copyWith(
      displaySmall: const TextStyle(
        fontSize: 30,
        fontWeight: FontWeight.w800,
        color: AppColors.ink,
        letterSpacing: -0.8,
      ),
      headlineSmall: const TextStyle(
        fontSize: 24,
        fontWeight: FontWeight.w700,
        color: AppColors.ink,
        letterSpacing: -0.4,
      ),
      titleLarge: const TextStyle(
        fontSize: 20,
        fontWeight: FontWeight.w700,
        color: AppColors.ink,
      ),
      bodyLarge: const TextStyle(
        fontSize: 16,
        fontWeight: FontWeight.w500,
        color: AppColors.ink,
      ),
      bodyMedium: const TextStyle(
        fontSize: 14,
        height: 1.45,
        color: AppColors.inkMuted,
      ),
      labelLarge: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
      labelMedium: const TextStyle(
        fontSize: 12,
        fontWeight: FontWeight.w700,
        letterSpacing: 0.8,
        color: AppColors.inkMuted,
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.surfaceMuted,
      contentPadding: const EdgeInsets.symmetric(horizontal: 18, vertical: 18),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(18),
        borderSide: BorderSide.none,
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(18),
        borderSide: BorderSide.none,
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(18),
        borderSide: const BorderSide(color: AppColors.primary, width: 1.5),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(18),
        borderSide: const BorderSide(color: AppColors.error),
      ),
      focusedErrorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(18),
        borderSide: const BorderSide(color: AppColors.error, width: 1.5),
      ),
      hintStyle: const TextStyle(color: AppColors.inkMuted, fontSize: 15),
      prefixIconColor: AppColors.inkMuted,
      suffixIconColor: AppColors.inkMuted,
    ),
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: AppColors.primary,
        foregroundColor: AppColors.onPrimary,
        elevation: 0,
        padding: const EdgeInsets.symmetric(vertical: 18),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
        textStyle: const TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
      ),
    ),
    cardTheme: CardThemeData(
      color: AppColors.surfaceRaised,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(28)),
    ),
  );
}
