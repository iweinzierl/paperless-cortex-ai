import 'package:flutter/material.dart';

class TailwindColors {
  // Primary
  static const Color primary = Color(0xff0051ae);
  static const Color onPrimary = Color(0xffffffff);
  static const Color primaryContainer = Color(0xff0969da);
  static const Color onPrimaryContainer = Color(0xffecefff);
  static const Color primaryFixed = Color(0xffd8e2ff);
  static const Color onPrimaryFixed = Color(0xff001a41);
  static const Color onPrimaryFixedVariant = Color(0xff004493);
  static const Color primaryFixedDim = Color(0xffadc6ff);
  static const Color inversePrimary = Color(0xffadc6ff);

  // Secondary
  static const Color secondary = Color(0xff5a5f66);
  static const Color onSecondary = Color(0xffffffff);
  static const Color secondaryContainer = Color(0xffdee3eb);
  static const Color onSecondaryContainer = Color(0xff60656c);
  static const Color secondaryFixed = Color(0xffdee3eb);
  static const Color onSecondaryFixed = Color(0xff171c22);
  static const Color onSecondaryFixedVariant = Color(0xff42474e);
  static const Color secondaryFixedDim = Color(0xffc2c7cf);

  // Tertiary
  static const Color tertiary = Color(0xff006326);
  static const Color onTertiary = Color(0xffffffff);
  static const Color tertiaryContainer = Color(0xff0d7e34);
  static const Color onTertiaryContainer = Color(0xffc3ffc4);
  static const Color tertiaryFixed = Color(0xff94f99f);
  static const Color onTertiaryFixed = Color(0xff002108);
  static const Color onTertiaryFixedVariant = Color(0xff00531e);
  static const Color tertiaryFixedDim = Color(0xff78dc86);

  // Surface
  static const Color surface = Color(0xfff8f9fb);
  static const Color onSurface = Color(0xff191c1e);
  static const Color surfaceVariant = Color(0xffe0e3e5);
  static const Color onSurfaceVariant = Color(0xff424753);
  static const Color surfaceContainerLowest = Color(0xffffffff);
  static const Color surfaceContainerLow = Color(0xfff2f4f6);
  static const Color surfaceContainer = Color(0xffeceef0);
  static const Color surfaceContainerHigh = Color(0xffe6e8ea);
  static const Color surfaceContainerHighest = Color(0xffe0e3e5);
  static const Color surfaceDim = Color(0xffd8dadc);
  static const Color surfaceBright = Color(0xfff8f9fb);
  static const Color inverseSurface = Color(0xff2d3133);
  static const Color inverseOnSurface = Color(0xffeff1f3);
  static const Color surfaceTint = Color(0xff005bc0);

  // Outline
  static const Color outline = Color(0xff727785);
  static const Color outlineVariant = Color(0xffc2c6d6);

  // Error
  static const Color error = Color(0xffba1a1a);
  static const Color onError = Color(0xffffffff);
  static const Color errorContainer = Color(0xffffdad6);
  static const Color onErrorContainer = Color(0xff93000a);

  // Background
  static const Color background = Color(0xfff8f9fb);
  static const Color onBackground = Color(0xff191c1e);
}

ThemeData buildAppTheme() {
  return ThemeData(
    useMaterial3: true,
    fontFamily: 'Inter',
    colorScheme: const ColorScheme(
      brightness: Brightness.light,
      primary: TailwindColors.primary,
      onPrimary: TailwindColors.onPrimary,
      primaryContainer: TailwindColors.primaryContainer,
      onPrimaryContainer: TailwindColors.onPrimaryContainer,
      secondary: TailwindColors.secondary,
      onSecondary: TailwindColors.onSecondary,
      secondaryContainer: TailwindColors.secondaryContainer,
      onSecondaryContainer: TailwindColors.onSecondaryContainer,
      tertiary: TailwindColors.tertiary,
      onTertiary: TailwindColors.onTertiary,
      tertiaryContainer: TailwindColors.tertiaryContainer,
      onTertiaryContainer: TailwindColors.onTertiaryContainer,
      error: TailwindColors.error,
      onError: TailwindColors.onError,
      errorContainer: TailwindColors.errorContainer,
      onErrorContainer: TailwindColors.onErrorContainer,
      surface: TailwindColors.surface,
      onSurface: TailwindColors.onSurface,
      surfaceContainerHighest: TailwindColors.surfaceContainerHighest,
      outline: TailwindColors.outline,
      outlineVariant: TailwindColors.outlineVariant,
    ),
    scaffoldBackgroundColor: TailwindColors.surface,
  );
}
